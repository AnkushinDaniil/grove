import { useEffect, useRef } from "react";

/**
 * Keeps a ref pointing at the latest value across renders, so effects can
 * read current callbacks/values without listing them in a dependency array
 * (avoiding effect re-runs caused only by inline-function identity churn).
 */
export function useLatestRef<T>(value: T) {
  const ref = useRef(value);
  useEffect(() => {
    ref.current = value;
  });
  return ref;
}
