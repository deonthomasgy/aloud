(function (root, factory) {
  const api = factory();
  if (typeof module === "object" && module.exports) module.exports = api;
  root.AloudHighlight = api;
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  "use strict";

  const ALNUM = /[\p{L}\p{N}]/u;

  function normalizeToken(value) {
    return Array.from(String(value || "").toLocaleLowerCase())
      .filter(ch => ALNUM.test(ch))
      .join("");
  }

  function canonicalRoman(value) {
    let remaining = value;
    let roman = "";
    for (const part of [[10, "X"], [9, "IX"], [5, "V"], [4, "IV"], [1, "I"]]) {
      while (remaining >= part[0]) {
        roman += part[1];
        remaining -= part[0];
      }
    }
    return roman;
  }

  function romanListValue(roman) {
    const upper = String(roman || "").toUpperCase();
    if (!/^[IVX]+$/.test(upper)) return null;
    const values = { I: 1, V: 5, X: 10 };
    let total = 0;
    for (let i = 0; i < upper.length; i++) {
      const value = values[upper[i]];
      total += i + 1 < upper.length && value < values[upper[i + 1]] ? -value : value;
    }
    return total >= 1 && total <= 39 && canonicalRoman(total) === upper ? total : null;
  }

  function numberWord(value) {
    const underTwenty = [
      "", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine",
      "ten", "eleven", "twelve", "thirteen", "fourteen", "fifteen", "sixteen",
      "seventeen", "eighteen", "nineteen",
    ];
    if (value < underTwenty.length) return underTwenty[value];
    const tens = ["", "", "twenty", "thirty"];
    const remainder = value % 10;
    return tens[Math.floor(value / 10)] + (remainder ? "-" + underTwenty[remainder] : "");
  }

  function finiteNumber(value, fallback) {
    const n = Number(value);
    return Number.isFinite(n) ? n : fallback;
  }

  function repairTimings(timestamps) {
    const repaired = (timestamps || []).map((item, index) => {
      const previousStart = index ? finiteNumber(timestamps[index - 1].start_time, 0) : 0;
      const start = Math.max(0, finiteNumber(item.start_time, previousStart));
      return {
        word: String(item.word || ""),
        start_time: start,
        end_time: finiteNumber(item.end_time, start),
      };
    });

    for (let i = 0; i < repaired.length; i++) {
      if (i > 0 && repaired[i].start_time < repaired[i - 1].start_time) {
        repaired[i].start_time = repaired[i - 1].start_time;
      }
      const next = repaired[i + 1];
      if (repaired[i].end_time <= repaired[i].start_time) {
        repaired[i].end_time = next && next.start_time > repaired[i].start_time
          ? next.start_time
          : repaired[i].start_time + 0.25;
      }
      if (next && repaired[i].end_time > next.start_time && next.start_time > repaired[i].start_time) {
        repaired[i].end_time = next.start_time;
      }
    }
    return repaired;
  }

  function matchedRange(text, start, target) {
    let normalized = "";
    let matchStart = -1;
    for (let i = start; i < text.length;) {
      const codePoint = text.codePointAt(i);
      const ch = String.fromCodePoint(codePoint);
      const width = ch.length;
      if (ALNUM.test(ch)) {
        if (matchStart < 0) matchStart = i;
        normalized += ch.toLocaleLowerCase();
        if (!target.startsWith(normalized)) return null;
        if (normalized === target) return { start: matchStart, end: i + width };
      }
      i += width;
    }
    return null;
  }

  function findRomanMarker(text, start, end, target) {
    const source = text.slice(start, end);
    const marker = /\(([ivx]+)\)/i.exec(source);
    if (!marker) return null;
    const value = romanListValue(marker[1]);
    if (value === null) return null;
    const spoken = normalizeToken(numberWord(value));
    if (spoken !== target && !spoken.startsWith(target)) return null;
    const markerStart = start + marker.index;
    return { start: markerStart, end: markerStart + marker[0].length };
  }

  function findWord(text, cursor, target) {
    let i = cursor;
    while (i < text.length) {
      while (i < text.length && /\s/u.test(text[i])) i++;
      if (i >= text.length) return null;

      let end = i;
      while (end < text.length && !/\s/u.test(text[end])) end++;
      const romanMarker = findRomanMarker(text, i, end, target);
      if (romanMarker) return romanMarker;
      const sourceToken = normalizeToken(text.slice(i, end));
      if (
        sourceToken === target ||
        sourceToken.startsWith(target) ||
        target.startsWith(sourceToken)
      ) {
        const range = matchedRange(text, i, target);
        if (range) return range;
      }
      i = end;
    }
    return null;
  }

  function timestampFallback(timestamps) {
    let displayText = "";
    const ranges = [];
    for (const timestamp of timestamps) {
      const word = String(timestamp.word || "");
      if (!word) {
        ranges.push(null);
        continue;
      }
      const attaches = /^[,.;:!?%)}\]”’]/u.test(word);
      if (displayText && !attaches) displayText += " ";
      const start = displayText.length;
      displayText += word;
      ranges.push({ start, end: displayText.length });
    }
    return { displayText, ranges, fallback: true, hits: 0 };
  }

  function alignText(text, timestamps) {
    const displayText = String(text || "");
    if (!displayText || !(timestamps || []).length) {
      return { displayText, ranges: [], fallback: false, hits: 0 };
    }

    const ranges = [];
    let cursor = 0;
    let hits = 0;
    for (const timestamp of timestamps) {
      const word = String(timestamp.word || "");
      const target = normalizeToken(word);
      let range = null;
      if (!target) {
        const start = word ? displayText.indexOf(word, cursor) : -1;
        if (start >= 0) range = { start, end: start + word.length };
      } else {
        range = findWord(displayText, cursor, target);
      }
      ranges.push(range);
      if (range) {
        hits++;
        cursor = range.end;
      }
    }

    const threshold = Math.max(1, Math.floor(timestamps.length * 2 / 3));
    if (hits < threshold) return timestampFallback(timestamps);
    return { displayText, ranges, fallback: false, hits };
  }

  function estimateTimings(text, duration) {
    const source = String(text || "");
    const totalDuration = Math.max(0.25, finiteNumber(duration, 0));
    const matches = Array.from(source.matchAll(/\S+/gu));
    if (!matches.length) return [];

    const weights = matches.map(match => {
      const letters = Math.max(1, normalizeToken(match[0]).length);
      const pause = /[.!?]["'”’)]*$/u.test(match[0]) ? 3 : /[,;:]["'”’)]*$/u.test(match[0]) ? 1.5 : 0;
      return letters + pause;
    });
    const totalWeight = weights.reduce((sum, weight) => sum + weight, 0);
    const lead = Math.min(0.15, totalDuration * 0.03);
    const usable = Math.max(0.1, totalDuration - lead);
    let elapsed = lead;

    return matches.map((match, index) => {
      const start = elapsed;
      elapsed += usable * weights[index] / totalWeight;
      return {
        word: match[0],
        start_time: start,
        end_time: index === matches.length - 1 ? totalDuration : elapsed,
        estimated: true,
      };
    });
  }

  function needsEstimatedTimings(timings, duration) {
    if (!timings || !timings.length) return true;
    const clipDuration = finiteNumber(duration, 0);
    if (clipDuration <= 0) return false;
    const lastEnd = finiteNumber(timings[timings.length - 1].end_time, 0);
    return clipDuration - lastEnd > Math.max(1.5, clipDuration * 0.25);
  }

  function activeIndex(timings, currentTime, duration) {
    if (!timings || !timings.length) return -1;
    const time = Math.max(0, finiteNumber(currentTime, 0));
    const clipDuration = finiteNumber(duration, Infinity);
    if (time >= clipDuration - 0.02) return -1;
    if (time <= timings[0].start_time) return 0;

    let active = 0;
    for (let i = 1; i < timings.length; i++) {
      if (time < timings[i].start_time) break;
      active = i;
    }
    const last = timings[timings.length - 1];
    if (active === timings.length - 1 && time > last.end_time + 0.05) return -1;
    return active;
  }

  return {
    activeIndex,
    alignText,
    estimateTimings,
    needsEstimatedTimings,
    normalizeToken,
    repairTimings,
  };
});
