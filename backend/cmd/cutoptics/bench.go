package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/size-module/backend/internal/optimizer"
	"github.com/size-module/backend/internal/optimizer/bench"
)

// runBench implements the "bench" subcommand:
//
//	go run ./cmd/cutoptics bench -dir testdata/benchmarks -random 6
//	go run ./cmd/cutoptics bench -check
//	go run ./cmd/cutoptics bench -update
//
// It exits non-zero when -check finds a regression, which makes it usable as a
// CI quality gate.
func runBench(args []string) int {
	fs := flag.NewFlagSet("bench", flag.ExitOnError)
	dir := fs.String("dir", "testdata/benchmarks", "directory with instance JSON files (missing is fine)")
	random := fs.Int("random", 0, "number of deterministic random instances to generate")
	seed := fs.Uint64("seed", bench.DefaultSeed, "seed for generated instances")
	solversFlag := fs.String("solvers", "", "comma separated solver names (default: every registered solver)")
	budget := fs.Int("budget", bench.DefaultBudgetMS, "time budget per solve, in ms")
	check := fs.Bool("check", false, "fail when a solver's mean waste is worse than the golden file")
	update := fs.Bool("update", false, "write the golden file from this run")
	goldenFlag := fs.String("golden", "", "golden file path (default: <dir>/golden.json)")
	tolerance := fs.Float64("tolerance", bench.GoldenTolerancePct, "allowed regression of the mean objective score in -check mode")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	goldenPath := *goldenFlag
	if goldenPath == "" {
		if *dir == "" {
			goldenPath = "golden.json"
		} else {
			goldenPath = filepath.Join(*dir, "golden.json")
		}
	}

	registry := optimizer.DefaultRegistry()
	solverNames := registry.Names()
	if *solversFlag != "" {
		solverNames = splitTrim(*solversFlag)
	}

	var instances []bench.Instance
	if *dir != "" {
		loaded, err := bench.LoadDir(*dir)
		switch {
		case err == nil:
			instances = append(instances, loaded...)
		case os.IsNotExist(err):
			// No committed instances: generated ones are enough.
		default:
			fmt.Fprintf(os.Stderr, "bench: loading %s: %v\n", *dir, err)
			return 1
		}
	}
	for i := 0; i < *random; i++ {
		instances = append(instances, bench.Generate(*seed, i))
	}
	if len(instances) == 0 {
		fmt.Fprintln(os.Stderr, "bench: no instances to run (use -dir or -random)")
		return 2
	}

	fmt.Printf("Running %d instance(s) with %d solver(s), budget %d ms\n\n",
		len(instances), len(solverNames), *budget)
	rows := bench.Run(context.Background(), registry, instances, solverNames, *budget)
	bench.Report(os.Stdout, rows)
	totals := bench.Aggregate(rows)

	exitCode := 0

	if *update {
		if err := bench.SaveGolden(goldenPath, totals); err != nil {
			fmt.Fprintf(os.Stderr, "bench: writing golden file: %v\n", err)
			return 1
		}
		fmt.Printf("\nGolden file updated: %s\n", goldenPath)
	}

	if *check {
		golden, err := bench.LoadGolden(goldenPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bench: reading golden file: %v\n", err)
			return 1
		}
		problems := bench.Compare(golden, totals, *tolerance)
		if len(problems) == 0 {
			fmt.Printf("\nGolden check passed (tolerance %.2f score points).\n", *tolerance)
		} else {
			fmt.Println("\nGolden check FAILED:")
			for _, problem := range problems {
				fmt.Printf("  - %s\n", problem)
			}
			exitCode = 1
		}
	}

	return exitCode
}

func splitTrim(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
