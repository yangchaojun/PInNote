/**
 * Tiny shared test kit: a module-level array both the test and the
 * vi.mock("./lib/api") factory can import (the factory cannot close over
 * test-file variables), so API-mock calls record in a single order list for
 * flush-ordering assertions.
 */
export const events: string[] = [];

/**
 * Renderer->Go events recorded by the mocked Wails runtime, so an ack can be
 * asserted without reaching into the module mock from the test file.
 */
export const emitted: Array<{ name: string; data: unknown }> = [];
