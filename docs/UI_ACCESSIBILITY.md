# Keyboard, focus and responsive UI acceptance (M3.11)

## Behavior

- Version actions use a labelled native modal dialog. Opening focuses its named
  close button; browser modal behavior confines keyboard focus and makes the
  background inert. Escape, close and backdrop dismissal restore the opener, or
  the versions heading if the opener was removed. Body scrolling is restored on
  cleanup. Closing pending work does not cancel a server deployment or reopen a
  dismissed modal when its response arrives.
- A skip link targets the main landmark. Client-side route changes focus the new
  main content (or the public page heading). Navigation identifies the current
  page. Version entries are buttons, so Enter and Space work without custom
  pointer-only handlers.
- After a submission disables the composer, focus returns to the composer or
  same-request retry button only if it would otherwise be left on the body.
  It does not pull focus away from another control.
- Shift+Enter inserts a newline; composing an IME character or holding Enter does
  not trigger submission. History updates no longer scroll the whole document.
- Mobile versions start collapsed in a native, keyboard-operable disclosure.
  Users can open/close it at any width. Narrow layouts use ordinary page scrolling
  instead of stacked, constrained sidebar/chat scroll areas.
- Long domains, version IDs, status text and action rows wrap without horizontal
  overflow. The dialog has one viewport-bounded scrolling area. Buttons have
  44px minimum height; modal close has a 44px square target.
- Default and primary control colors are explicit under either system color
  preference. Focus indicators override base-style resets. Primary colors and
  muted text were darkened; the browser audit checks primary-button text contrast
  of at least 4.5:1. Reduced-motion preferences disable transitions/animations.

The supported preview/publish/draft protocols are unchanged. Native dialog and
disclosure behavior is used rather than a new UI framework.

## Repeatable checks

From `pagewright/ui`, with an installed, compatible Playwright/Firefox pair:

```bash
npm run test:contracts
npm run lint -- --max-warnings=0
npm run build
node test/draftBrowser.mjs /absolute/path/to/playwright/index.mjs /absolute/path/to/firefox --accessibility
node test/draftBrowser.mjs /absolute/path/to/playwright/index.mjs /absolute/path/to/firefox
```

The audit reuses the isolated static-server/mocked-gateway harness from M3.10.
It tests dashboard → chat → modal with deliberately long domains and IDs at:

| Viewport | Purpose |
| --- | --- |
| 1280 × 900 | Desktop |
| 768 × 1024 | Tablet/breakpoint |
| 390 × 844 | Narrow portrait |
| 320 × 568 | Small portrait/reflow |
| 640 × 450 | Short landscape/reflow |

Assertions cover keyboard skip/navigation, mobile disclosure, route focus,
Enter/Space activation, Tab/Shift+Tab containment, inert background, Escape/close
restoration, pending-work dismissal, scroll-lock cleanup, IME/newline safety,
button size, page/dialog horizontal overflow, dialog bounds, visible focus,
primary contrast and reduced motion with a dark system preference.
Synthetic screenshots are written to `/tmp/pagewright-m311-*.png` for visual
inspection, including collapsed-mobile chat and viewport-only dialogs.

This run used cached Playwright 1.58.2 with its matching Firefox revision 1509.
The newer preinstalled driver could launch that browser but could not resize its
viewport; no dependencies were installed to resolve the mismatch. Use matching
driver/browser revisions when repeating viewport tests.

The existing rendered draft-recovery test remains a separate regression for
expiry/login/retry, publication feedback, dashboard refresh and storage handling.
Production Compose smoke separately checks built-UI startup, nginx crash handling
and preserved data across recreation. Neither browser harness makes provider calls.

## Scope of acceptance

This is a focused keyboard, semantic and responsive-layout audit with screenshot
review, not WCAG certification. Screen-reader speech, real-device touch/virtual
keyboards, actual browser zoom, forced-colors mode and Safari/Chromium were not
verified by this Firefox run. Synthetic composition events are not a real IME
installation. M3.12 still owns the complete browser journey against real services;
M4 owns remote-pilot security. No backend, schema, provider or hosting-policy
changes are part of M3.11.
