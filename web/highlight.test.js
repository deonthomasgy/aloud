const test = require("node:test");
const assert = require("node:assert/strict");

const {
  activeIndex,
  alignText,
  estimateTimings,
  needsEstimatedTimings,
  normalizeToken,
  repairTimings,
} = require("./highlight.js");

test("normalizes smart punctuation without losing letters", () => {
  assert.equal(normalizeToken("“Licensee.”"), "licensee");
  assert.equal(normalizeToken("know-how"), "knowhow");
});

test("aligns the quoted paragraph while preserving its original text", () => {
  const text = "\"Nothing in this Agreement transfers ownership of the Software, databases.\"";
  const timestamps = [
    { word: "Nothing" }, { word: "in" }, { word: "this" }, { word: "Agreement" },
    { word: "transfers" }, { word: "ownership" }, { word: "of" }, { word: "the" },
    { word: "Software" }, { word: "," }, { word: "databases" }, { word: "." },
  ];
  const aligned = alignText(text, timestamps);

  assert.equal(aligned.displayText, text);
  assert.equal(aligned.fallback, false);
  assert.equal(aligned.ranges.filter(Boolean).length, timestamps.length);
  assert.equal(text.slice(aligned.ranges[0].start, aligned.ranges[0].end), "Nothing");
  assert.equal(text.slice(aligned.ranges[9].start, aligned.ranges[9].end), ",");
});

test("aligns a merged spoken word across source hyphenation", () => {
  const text = "The long- standing tradition";
  const aligned = alignText(text, [
    { word: "The" },
    { word: "longstanding" },
    { word: "tradition" },
  ]);

  assert.equal(aligned.fallback, false);
  assert.equal(text.slice(aligned.ranges[1].start, aligned.ranges[1].end), "long- standing");
});

test("aligns separately spoken acronym letters", () => {
  const text = "The U.S. Licensor";
  const aligned = alignText(text, [
    { word: "The" },
    { word: "U" },
    { word: "S" },
    { word: "Licensor" },
  ]);

  assert.equal(aligned.fallback, false);
  assert.equal(text.slice(aligned.ranges[1].start, aligned.ranges[1].end), "U");
  assert.equal(text.slice(aligned.ranges[2].start, aligned.ranges[2].end), "S");
});

test("aligns spoken numbers to parenthesized Roman list markers", () => {
  const text = "Use (i) modifications, (ii) combinations, and (iv) replacements.";
  const aligned = alignText(text, [
    { word: "Use" },
    { word: "one" },
    { word: "modifications" },
    { word: "two" },
    { word: "combinations" },
    { word: "and" },
    { word: "four" },
    { word: "replacements" },
  ]);

  assert.equal(aligned.displayText, text);
  assert.equal(aligned.fallback, false);
  assert.equal(text.slice(aligned.ranges[1].start, aligned.ranges[1].end), "(i)");
  assert.equal(text.slice(aligned.ranges[3].start, aligned.ranges[3].end), "(ii)");
  assert.equal(text.slice(aligned.ranges[6].start, aligned.ranges[6].end), "(iv)");
});

test("aligns a compound number word to a higher Roman marker", () => {
  const text = "Clause (xxi) applies.";
  const aligned = alignText(text, [
    { word: "Clause" },
    { word: "twenty-one" },
    { word: "applies" },
  ]);

  assert.equal(aligned.fallback, false);
  assert.equal(text.slice(aligned.ranges[1].start, aligned.ranges[1].end), "(xxi)");
});

test("does not treat a bare i or alphabetic clause c as a Roman marker", () => {
  const aligned = alignText("I referenced clause (c).", [{ word: "one" }]);
  assert.equal(aligned.fallback, true);
});

test("repairs missing, overlapping, and non-monotonic times", () => {
  const repaired = repairTimings([
    { word: "one", start_time: 0.2, end_time: 0 },
    { word: "two", start_time: 0.8, end_time: 2 },
    { word: "three", start_time: 0.7, end_time: 0.7 },
  ]);

  assert.deepEqual(
    repaired.map(item => [item.start_time, item.end_time]),
    [[0.2, 0.8], [0.8, 2], [0.8, 1.05]],
  );
});

test("estimates full-clip timings when captions are empty", () => {
  const estimated = estimateTimings("Nothing in this Agreement.", 4);

  assert.equal(estimated.length, 4);
  assert.equal(estimated[0].estimated, true);
  assert.equal(estimated.at(-1).end_time, 4);
  assert.ok(estimated[1].start_time >= estimated[0].end_time);
});

test("detects empty or truncated captions", () => {
  assert.equal(needsEstimatedTimings([], 10), true);
  assert.equal(needsEstimatedTimings([{ end_time: 3 }], 10), true);
  assert.equal(needsEstimatedTimings([{ end_time: 9.4 }], 10), false);
});

test("resolves highlights before start, across gaps, and after completion", () => {
  const timings = [
    { start_time: 0.2, end_time: 0.5 },
    { start_time: 1, end_time: 1.4 },
  ];

  assert.equal(activeIndex(timings, 0, 2), 0);
  assert.equal(activeIndex(timings, 0.8, 2), 0);
  assert.equal(activeIndex(timings, 1.2, 2), 1);
  assert.equal(activeIndex(timings, 1.8, 2), -1);
  assert.equal(activeIndex(timings, 2, 2), -1);
});
