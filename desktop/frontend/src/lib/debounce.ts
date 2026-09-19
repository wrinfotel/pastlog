// Debounce for keystroke-driven queries (~250 ms, spec §4.3).
export function debounce<A extends unknown[]>(fn: (...args: A) => void, ms = 250): (...args: A) => void {
  let timer: ReturnType<typeof setTimeout> | undefined;
  return (...args: A) => {
    if (timer !== undefined) clearTimeout(timer);
    timer = setTimeout(() => fn(...args), ms);
  };
}
