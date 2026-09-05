// jsdom lacks the geometry APIs ProseMirror's view relies on (coordsAtPos,
// scrollToSelection after focus). Stub them with zero-rects: the editor works,
// nothing has a real screen position under test.
function zeroRect(): DOMRect {
  return {
    top: 0,
    left: 0,
    bottom: 0,
    right: 0,
    width: 0,
    height: 0,
    x: 0,
    y: 0,
    toJSON: () => ({}),
  } as DOMRect;
}

function zeroRectList(): DOMRectList {
  return [zeroRect()] as unknown as DOMRectList;
}

if (!Element.prototype.getClientRects) {
  Element.prototype.getClientRects = function () {
    return zeroRectList();
  };
  Element.prototype.getBoundingClientRect = function () {
    return zeroRect();
  };
}
Element.prototype.scrollIntoView ??= () => undefined;

// ProseMirror measures text positions through document.createRange(); jsdom's
// Range has none of the geometry methods real browsers provide.
if (typeof Range !== "undefined" && !Range.prototype.getClientRects) {
  Range.prototype.getClientRects = function () {
    return zeroRectList();
  };
  Range.prototype.getBoundingClientRect = function () {
    return zeroRect();
  };
}
