/**
 * Interim vehicle data convention (plan.md 3.3): item.description may open
 * with "Key: value" lines, then a blank line, then prose. The parser is
 * tolerant: no header means the whole description is prose.
 */
export interface ParsedDescription {
  specs: Map<string, string>;
  body: string;
}

const HEADER_LINE = /^([A-Za-z][A-Za-z ]{0,30}):\s*(\S.*)$/;

export function parseDescription(description: string): ParsedDescription {
  const specs = new Map<string, string>();
  const lines = description.replace(/\r\n/g, "\n").split("\n");

  let index = 0;
  for (; index < lines.length; index++) {
    const line = lines[index].trim();
    if (line === "") break;
    const match = HEADER_LINE.exec(line);
    if (!match) {
      // The first non-matching line means there was no header at all.
      return { specs: new Map(), body: description.trim() };
    }
    specs.set(normaliseKey(match[1]), match[2].trim());
  }

  if (specs.size === 0) return { specs, body: description.trim() };
  return { specs, body: lines.slice(index + 1).join("\n").trim() };
}

function normaliseKey(key: string): string {
  const trimmed = key.trim().toLowerCase();
  return trimmed.charAt(0).toUpperCase() + trimmed.slice(1);
}

/** Cards show the first three available of these, in this order. */
const CARD_SPEC_ORDER = ["Mileage", "Engine", "Transmission", "Fuel"];

export function cardSpecs(specs: Map<string, string>): Array<[label: string, value: string]> {
  const rows: Array<[string, string]> = [];
  for (const key of CARD_SPEC_ORDER) {
    const value = specs.get(key);
    if (value) rows.push([key, value]);
    if (rows.length === 3) break;
  }
  return rows;
}
