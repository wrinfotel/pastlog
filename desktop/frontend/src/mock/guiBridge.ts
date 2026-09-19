// Dev-only mock of wailsjs/go/main/guiBridge (see app.ts).
export async function PickSavePath() {
  return null; // pretend the user cancelled the dialog
}

export async function OpenExternal() {
  /* noop in the browser */
}
