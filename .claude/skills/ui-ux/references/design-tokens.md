# Design tokens — financial-tracker

Source: Canva AI design brief for a calm/premium/trustworthy financial tracker.
All values also live as CSS custom properties in `tokens.css` in this directory.

## Brand colors

| Role      | Name       | Hex       | Use                                          |
|-----------|-----------|-----------|-----------------------------------------------|
| Primary   | Deep Navy  | `#1E3A5F` | headers, primary buttons, key highlights      |
| Secondary | Teal       | `#2A9D8F` | secondary actions, progress, positive trends  |
| Accent    | Soft Mint  | `#7BDCB5` | success states, gentle emphasis               |
| Accent 2  | Sky Blue   | `#4DA3FF` | charts, links, informational highlights       |

## Neutrals

| Role           | Name             | Hex       | Use                              |
|----------------|------------------|-----------|-----------------------------------|
| Background     | Off White        | `#F7F9FC` | page background                   |
| Surface        | White            | `#FFFFFF` | cards, panels, modals             |
| Border         | Light Gray Blue  | `#E3EAF2` | dividers, card borders            |
| Text Primary   | Charcoal Navy    | `#102A43` | main text                         |
| Text Secondary | Slate            | `#627D98` | labels, helper text               |
| Muted          | Cool Gray        | `#94A3B8` | disabled text, low-priority UI    |

## Semantic colors

| Role              | Name        | Hex       | Use                                  |
|-------------------|-------------|-----------|----------------------------------------|
| Income / Positive | Green       | `#16A34A` | incoming money, positive balance      |
| Expense / Negative| Red         | `#DC2626` | spending, over-budget alerts          |
| Warning           | Amber       | `#F59E0B` | caution, approaching limits           |
| Info              | Blue        | `#2563EB` | tips, neutral info                    |
| Success soft      | Light Green | `#DCFCE7` | success banners, chips                |
| Error soft        | Light Red   | `#FEE2E2` | error banners, validation             |

## Chart colors

| Series             | Hex       |
|--------------------|-----------|
| Income             | `#16A34A` |
| Spending           | `#DC2626` |
| Savings            | `#2563EB` |
| Budget progress    | `#2A9D8F` |
| Forecast/projection| `#7C3AED` |

Chart types: line (balance trend), bar (monthly spending), donut (category
split), progress bar (savings goals). Soft gridlines, short labels, clear
tooltips, rounded bars/lines, legends only when needed.

## Radii

| Element        | Radius  |
|----------------|---------|
| Cards          | 16px    |
| Buttons/inputs | 12px    |
| Chips/pills    | 999px   |

## Shadow

Single soft shadow, used sparingly (cards/modals, not everywhere):

```css
box-shadow: 0 8px 24px rgba(16, 42, 67, 0.08);
```

## Spacing (8px base grid)

`8px 16px 24px 32px 40px 48px`

## Typography

- Headings: Inter / Sora / Manrope
- Body: Inter / Source Sans 3 / Roboto
- Page title: 28-32px bold
- Section title: 18-22px semibold
- Body: 14-16px regular
- Labels: 12-13px medium
- Numbers/balances: 20-28px bold, tabular numerals, right-aligned in lists

## Interaction

- Hover: subtle. Active: slightly darker.
- Transitions: 150-200ms, no flashy motion.
- Focus ring (inputs): `box-shadow: 0 0 0 3px rgba(42, 157, 143, 0.15)` (teal)

## Buttons

- Primary: bg Deep Navy, text White, hover slightly darker navy — main actions
  ("Add Transaction").
- Secondary: bg White, border Light Gray Blue, text Deep Navy or Teal.
- Accent: bg Teal, text White — positive actions ("Set Goal").

## Gradients (use sparingly — hero headers / key balance cards only)

- Navy → Teal: `#1E3A5F` → `#2A9D8F`
- Blue → Mint: `#2563EB` → `#7BDCB5`
- Light surface: `#FFFFFF` → `#F7F9FC`

## Accessibility

- Strong text/background contrast everywhere; avoid thin gray text on white.
- Never rely on color alone for profit/loss — pair with icon, sign, or label.
- Visible focus states on all interactive elements.
- Text sized for mobile/touch use.
