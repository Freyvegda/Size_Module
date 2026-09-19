import { useDispatch, useSelector } from 'react-redux'

import type { AppDispatch, RootState } from './store'

// Typed versions of the react-redux hooks.
export const useAppDispatch = useDispatch.withTypes<AppDispatch>()
export const useAppSelector = useSelector.withTypes<RootState>()
