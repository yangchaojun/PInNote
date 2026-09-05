import { describe, expect, it } from "vitest";
import { looksLikeMarkdown } from "./editorConfig";

describe("looksLikeMarkdown heuristic", () => {
  it("flags line-start block syntax", () => {
    expect(looksLikeMarkdown("## heading")).toBe(true);
    expect(looksLikeMarkdown("- item\n- other")).toBe(true);
    expect(looksLikeMarkdown("1. step\n2. step")).toBe(true);
    expect(looksLikeMarkdown("> quoted")).toBe(true);
    expect(looksLikeMarkdown("```js\ncode\n```")).toBe(true);
    expect(looksLikeMarkdown("see [docs](https://x.test)")).toBe(true);
  });
  it("leaves prose alone", () => {
    expect(looksLikeMarkdown("just a sentence about milk and eggs")).toBe(false);
    expect(looksLikeMarkdown("apple - a fruit (mid-line dash is fine)")).toBe(false);
  });
});
