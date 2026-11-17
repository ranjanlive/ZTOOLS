// Validates tool arguments against the JSON Schema subset the tool definitions use,
// applies defaults and reports clear errors to the agent.

import { ToolError } from "./util.js";

export function validateArgs(schema, input) {
  const args = input ?? {};
  if (typeof args !== "object" || Array.isArray(args)) {
    throw new ToolError("INVALID_ARGS", "Arguments must be a JSON object.");
  }
  const props = schema.properties || {};
  for (const key of Object.keys(args)) {
    if (!(key in props)) {
      throw new ToolError("INVALID_ARGS", `Unknown argument "${key}". Allowed: ${Object.keys(props).join(", ")}.`);
    }
  }
  const out = {};
  for (const [key, spec] of Object.entries(props)) {
    const value = args[key];
    if (value === undefined || value === null) {
      if (spec.default !== undefined) out[key] = structuredClone(spec.default);
      continue;
    }
    out[key] = coerce(key, spec, value);
  }
  for (const key of schema.required || []) {
    if (out[key] === undefined || out[key] === "") {
      throw new ToolError("INVALID_ARGS", `Missing required argument "${key}".`);
    }
  }
  return out;
}

function coerce(path, spec, value) {
  switch (spec.type) {
    case "string": {
      if (typeof value === "number" || typeof value === "boolean") value = String(value);
      if (typeof value !== "string") fail(path, "a string");
      if (spec.enum) {
        const match = spec.enum.find((e) => e.toLowerCase() === value.trim().toLowerCase());
        if (!match) fail(path, `one of: ${spec.enum.join(", ")}`);
        return match;
      }
      return value;
    }
    case "integer":
    case "number": {
      let n = typeof value === "string" && value.trim() !== "" ? Number(value) : value;
      if (typeof n !== "number" || !Number.isFinite(n)) fail(path, `a ${spec.type}`);
      if (spec.type === "integer") {
        if (!Number.isInteger(n)) fail(path, "an integer");
      }
      if (spec.minimum !== undefined && n < spec.minimum) fail(path, `>= ${spec.minimum}`);
      if (spec.maximum !== undefined && n > spec.maximum) fail(path, `<= ${spec.maximum}`);
      return n;
    }
    case "boolean": {
      if (value === "true") return true;
      if (value === "false") return false;
      if (typeof value !== "boolean") fail(path, "true or false");
      return value;
    }
    case "array": {
      if (!Array.isArray(value)) fail(path, "an array");
      if (spec.minItems !== undefined && value.length < spec.minItems) fail(path, `an array with at least ${spec.minItems} items`);
      if (spec.maxItems !== undefined && value.length > spec.maxItems) fail(path, `an array with at most ${spec.maxItems} items`);
      return spec.items ? value.map((v, i) => coerce(`${path}[${i}]`, spec.items, v)) : value;
    }
    case "object": {
      if (typeof value !== "object" || Array.isArray(value)) fail(path, "an object");
      return value;
    }
    default:
      return value;
  }
}

function fail(path, expected) {
  throw new ToolError("INVALID_ARGS", `Argument "${path}" must be ${expected}.`);
}
