# Model Metadata CNY Pricing Design

## Goal

Make the pricing section in the model metadata create/edit drawer follow the system CNY display configuration with the same semantics already used by the model pricing editor under system settings.

This change is limited to currency consistency. It does not remove, merge, or otherwise redesign the two model-pricing entry points.

## Existing Contract

- Billing stores fixed model prices in USD in `ModelPrice`.
- Token pricing stores currency-independent ratios in `ModelRatio`, `CompletionRatio`, `CacheRatio`, `ImageRatio`, `AudioRatio`, and `AudioCompletionRatio`.
- The configured CNY exchange rate means CNY per USD.
- The system-settings model pricing editor already displays CNY input while converting fixed prices back to USD on save.
- `TOKENS` and `CUSTOM` display modes continue to use the existing USD pricing-editor behavior in this change. Only CNY support is in scope.

## User Experience

When `general_setting.quota_display_type` is `CNY`:

- Fixed-price inputs display CNY values and a CNY/yen label instead of USD/dollar text.
- Token input and output price helpers display CNY per one million tokens.
- Ratio-mode calculated-price previews display converted CNY values.
- An informational message explains that CNY values are converted to USD when saved.
- A missing, zero, negative, or non-finite CNY exchange rate disables currency-valued editing and presents the existing invalid-rate message.

When the display type is not `CNY`, the drawer preserves its current USD behavior.

Pure ratio fields remain unchanged in every display mode because they are not currency amounts.

## Data Flow

### Fixed price per request

1. Read the stored USD value from `ModelPrice`.
2. Convert USD to CNY for the form using the configured exchange rate.
3. Let the administrator edit the CNY value.
4. Convert the submitted CNY value back to USD before updating `ModelPrice`.

### Token price input mode

1. Derive the canonical USD input price as `ModelRatio * 2` dollars per one million tokens.
2. Convert that USD price to CNY for display.
3. Convert an edited CNY input price back to USD before deriving `ModelRatio` as `USD input price / 2`.
4. Derive completion ratio from output price divided by input price. Because both prices use the same currency, the ratio remains currency-independent.

### Ratio input mode

1. Keep the entered model and completion ratios unchanged.
2. Derive canonical USD preview prices from the ratios.
3. Convert only the preview amounts to CNY.

## Implementation Boundaries

- Subscribe to `useSystemConfigStore` with narrow selectors for display type, USD exchange rate, and loading state.
- Reuse `formatDisplayPriceFromUSD`, `formatUSDPriceFromDisplay`, and `formatPricingNumber` from the established system-settings pricing implementation.
- Reuse existing translated CNY messages where possible. Any new user-facing keys must be synchronized across every locale.
- Keep backend APIs, database schema, option names, and billing calculations unchanged.
- Do not modify model endpoint configuration or consolidate pricing entry points in this change.

## Complete Currency-Sensitive Surface

The implementation must cover all of the following locations in the model metadata drawer:

1. Loading an existing per-request price.
2. Saving a per-request price.
3. Loading token input and completion prices from stored ratios.
4. Editing the token input price and deriving the stored model ratio.
5. Editing the token completion price and deriving the completion ratio.
6. Ratio-mode input-price preview.
7. Ratio-mode completion-price preview.
8. Fixed-price label, unit, placeholder, and description.
9. Price-mode label and token-price field labels/placeholders.
10. CNY conversion notice and invalid-exchange-rate state.
11. Reset behavior when opening the drawer for a new model or switching currency configuration.

## Validation

- Preserve the existing pricing-format USD/CNY round-trip tests.
- Add focused regression coverage for the model metadata drawer's fixed-price and ratio-derived CNY transformations.
- First reproduce the current CNY drawer showing USD labels as a failing browser assertion.
- Run the focused pricing tests, TypeScript typecheck, lint on touched files, formatting check, and production build.
- In the browser, verify both CNY and USD presentation, no framework overlay, no relevant console errors, and a real input interaction that updates the derived ratio or preview correctly.

## Out of Scope

- Choosing whether one of the two pricing entry points should be removed.
- Refactoring both entry points into a single shared visual component.
- Per-channel pricing or upstream cost accounting.
- Adding CNY support to expression literals, which remain canonical USD values.
- Changing custom-currency or token-display behavior.
