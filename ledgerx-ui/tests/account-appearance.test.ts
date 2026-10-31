import assert from "node:assert/strict";
import { afterEach, test } from "node:test";
import {
  readAppearance,
  saveAppearance,
} from "../src/lib/account-appearance.ts";

const originalStorage = Object.getOwnPropertyDescriptor(
  globalThis,
  "localStorage",
);

afterEach(() => {
  if (originalStorage)
    Object.defineProperty(globalThis, "localStorage", originalStorage);
  else Reflect.deleteProperty(globalThis, "localStorage");
});

function withStorage(value: string | null) {
  const data = new Map<string, string>(
    value === null ? [] : [["ledgerx.appearance.1", value]],
  );
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    value: {
      getItem: (key: string) => data.get(key) ?? null,
      setItem: (key: string, item: string) => {
        data.set(key, item);
      },
    },
  });
}

test("account appearance is retained without changing other accounts", () => {
  withStorage(null);
  const other = readAppearance(2);
  saveAppearance(1, { icon: "bank", color: "apricot" });
  assert.deepEqual(readAppearance(1), { icon: "bank", color: "apricot" });
  assert.deepEqual(readAppearance(2), other);
});

test("corrupt or outdated preferences fall back to a supported appearance", () => {
  for (const invalid of [
    "not-json",
    "null",
    '{"icon":"unknown","color":"sage"}',
    '{"icon":"bank","color":"obsolete"}',
  ]) {
    withStorage(invalid);
    assert.deepEqual(readAppearance(1), { icon: "wallet", color: "sage" });
  }
});

test("unavailable storage never breaks account rendering or creation", () => {
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    get: () => {
      throw new DOMException("Storage unavailable", "SecurityError");
    },
  });
  assert.deepEqual(readAppearance(1), { icon: "wallet", color: "sage" });
  assert.doesNotThrow(() => saveAppearance(1, { icon: "card", color: "sky" }));
});
