// Small identity palette for profile dots in the usage meter -- deliberately
// disjoint from the app's status hues (gray/amber/teal/blue/red/orange) and
// the lime brand accent, so a profile's color never reads as a status.
const PALETTE = ["#a78bfa", "#e879f9", "#67e8f9"] as const;

export function profileColor(profileId: string): string {
  let hash = 0;
  for (let i = 0; i < profileId.length; i++) {
    hash = (hash * 31 + profileId.charCodeAt(i)) | 0;
  }
  return PALETTE[Math.abs(hash) % PALETTE.length];
}
