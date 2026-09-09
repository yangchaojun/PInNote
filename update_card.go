package main

// updateCardCSS retints the framework built-in update card with PinNote
// design tokens (ADR-0003 Q3). The template derives everything from custom
// properties on :root, so redefining those is enough; its 13px base font
// already matches the pin windows.
const updateCardCSS = `
:root {
  --accent: #6f8ffa;
  --accent-dim: #6f8ffa33;
  --radius: 8px;
  --radius-sm: 6px;
}
@media (prefers-color-scheme: light) {
  :root { --bg: #fafafb; --surface: #ffffff; --surface-2: #f2f2f4; --border: #dcdce1; }
}
@media (prefers-color-scheme: dark) {
  :root {
    --bg: #1e1f22;
    --surface: #26272b;
    --surface-2: #2d2e33;
    --fg: #f5f5f7;
    --fg-dim: #b0b0b8;
    --border: #3a3b40;
    --accent-dim: #6f8ffa44;
  }
}
`
