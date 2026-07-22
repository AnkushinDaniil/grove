// Data-viz fill for the stats dashboard's plain magnitude bars/areas (by
// driver, by model, top nodes, skills). Deliberately its own hue -- not the
// lime brand accent (reserved for the one headline series, and for
// attention/focus chrome elsewhere -- see AttentionBadge), and not any
// status hue (reserved for genuinely status-shaped series, e.g. session
// outcomes or tool error rates -- see RollupMiniBar/ChecksPill). Same violet
// family as profileColor.ts's palette, which documents the identical
// "disjoint from status + accent" rule for per-profile identity dots.
//
// Verified via the dataviz skill's validator (scripts/validate_palette.js
// `contrast()`) at >=3:1 against every grove surface in both themes -- a
// single mode-agnostic value, unlike the CSS-variable pairs the rest of the
// app uses, so bars don't need a light/dark branch of their own.
export const VIZ_FILL = "#8b5cf6";
