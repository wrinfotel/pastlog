// Dev-only mock of wailsjs/runtime/runtime (see app.ts).
export function EventsOn(_name: string, _cb: unknown) {
  /* no progress events in mock mode */
}

export function EventsOff(_name: string) {}
