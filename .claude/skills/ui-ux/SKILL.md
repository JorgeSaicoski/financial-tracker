---
name: ui-ux
description: Use whenever writing or reviewing UI in web/ — any Svelte component, page, form, chart, or CSS. Applies the financial-tracker design system (colors, type, spacing, radii, shadows) so the app reads as one calm, trustworthy, premium product instead of a patchwork of one-off styles. Triggers on "add a page", "style this", "new component", "form", "button", "card", "chart", "dashboard", or any `.svelte`/`.css` edit under web/.
---

# Financial Tracker UI/UX

Design direction: **calm, premium, high-clarity**. Money data should feel easy to
scan, not stressful. Low-noise, pleasant for daily use, friendly without being
childish.

## Before styling anything

1. Read `references/design-tokens.md` for the full palette, type scale, spacing,
   radii, and shadow values.
2. Check whether `web/src/lib/styles/tokens.css` exists yet. If not, and you're
   about to add styled markup, copy `references/tokens.css` in as
   `web/src/lib/styles/tokens.css` and import it once globally (e.g. from
   `+layout.svelte` or `app.html`) rather than re-declaring colors per component.
3. Use the CSS custom properties from tokens.css (`var(--color-primary)`, etc.)
   in component `<style>` blocks — don't hardcode hex values inline.

## Rules

- **Background/surface**: page background is off-white (`--color-bg`), cards are
  white (`--color-surface`) with a light border (`--color-border`), not pure
  white-on-white.
- **Color roles**: Deep Navy = primary actions/headers. Teal = secondary actions,
  progress, positive trends. Green/Red = income/expense — but never color alone;
  pair with a `+`/`-` sign, icon, or label so colorblind users aren't relying on
  hue.
- **Radii**: cards 16px, buttons/inputs 12px, chips/pills 999px.
- **Shadows**: soft only — `0 8px 24px rgba(16, 42, 67, 0.08)`. No heavy/hard
  shadows.
- **Spacing**: 8px base grid (8/16/24/32/40/48). Generous white space over dense
  layouts.
- **Typography**: Inter (or Sora/Manrope for headings) as the sans stack. Page
  title 28-32px bold, section title 18-22px semibold, body 14-16px, labels
  12-13px medium, balances/amounts 20-28px bold. Use tabular numerals
  (`font-variant-numeric: tabular-nums`) and right-align currency values in
  lists so digits line up.
- **Buttons**: primary = navy bg/white text; secondary = white bg, border,
  navy/teal text; accent (positive actions like "Set Goal") = teal bg/white
  text. Hover/active states subtle; transitions 150-200ms, no flashy motion.
- **Forms**: white input bg, light border, 12px radius, teal focus ring
  (`box-shadow: 0 0 0 3px rgba(42, 157, 143, 0.15)`). Validation messages short;
  success = soft green, error = soft red, warning = amber.
- **Charts**: line for balance trend, bar for monthly spending, donut for
  category split, progress bar for savings goals. Soft gridlines, short labels,
  rounded bars/lines. Colors: income `#16A34A`, spending `#DC2626`, savings
  `#2563EB`, budget progress `#2A9D8F`, forecast `#7C3AED`. Also see the
  `dataviz` skill for general charting mechanics (form, accessible color
  contrast) — this skill's chart colors take precedence for financial-tracker.
- **Gradients**: allowed only in hero headers or a key balance card, never as
  the default for ordinary components (navy→teal or blue→mint).

## Accessibility (non-negotiable)

- Maintain strong text/background contrast; no thin gray text on white.
- Never use color as the only signal for profit/loss/status — add icon/label/sign.
- Visible focus states on every interactive element.
- Text large enough for mobile/touch targets.

## Priority order when laying out a page

Balance → income vs. expense summary → spending categories → recent
transactions → goals/savings progress → trend charts. Most important number
first.
