// Theme resolution contract for the inline bootstrap in theme.js, run in
// jsdom against the actual script text (not a re-implementation). jsdom's
// window has no matchMedia, which is precisely the environment theme.js
// must survive.
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { join } from "node:path";

function themePath(): string {
  try {
    return fileURLToPath(new URL("./theme.js", import.meta.url));
  } catch {
    // vitest inlines modules as data: URLs; from the web root the script
    // is a fixed relative hop away.
    return join(process.cwd(), "static", "theme.js");
  }
}

const themeScript = readFileSync(themePath(), "utf8");

let meta: HTMLMetaElement | undefined;

function runTheme(): { theme: string; meta: string | null } {
  // The script references bare localStorage/window/document, so it runs
  // against jsdom's own globals.
  new Function(themeScript)();
  return {
    theme: document.documentElement.dataset.theme ?? "",
    meta:
      document.querySelector('meta[name="theme-color"]') instanceof HTMLMetaElement
        ? document
            .querySelector('meta[name="theme-color"]')
            .content
        : null,
  };
}

describe("static/theme.js", () => {
  beforeEach(() => {
    localStorage.removeItem("annalist-theme");
    document.documentElement.removeAttribute("data-theme");
    meta = document.createElement("meta");
    meta.setAttribute("name", "theme-color");
    document.head.appendChild(meta);
  });

  afterEach(() => {
    meta?.remove();
    meta = undefined;
    delete window.matchMedia;
  });

  it("follows the stored dark choice", () => {
    localStorage.setItem("annalist-theme", "dark");
    const { theme, meta } = runTheme();
    expect(theme).toBe("dark");
    expect(meta).toBe("#211d19");
  });

  it("follows the stored light choice", () => {
    localStorage.setItem("annalist-theme", "light");
    const { theme, meta } = runTheme();
    expect(theme).toBe("light");
    expect(meta).toBe("#f5eee5");
  });

  it("follows a dark media preference when nothing is stored", () => {
    window.matchMedia = () => ({ matches: true });
    const { theme } = runTheme();
    expect(theme).toBe("dark");
  });

  it("falls back to light when the media preference is light", () => {
    window.matchMedia = () => ({ matches: false });
    const { theme } = runTheme();
    expect(theme).toBe("light");
  });

  it("a missing matchMedia falls back to light without an uncaught error", () => {
    // jsdom has no window.matchMedia, so the call throws inside the script
    // and the catch block applies light; the meta tag is left untouched.
    runTheme();
    expect(document.documentElement.dataset.theme).toBe("light");
    expect(meta?.getAttribute("content")).toBeNull();
  });

  it("applies the theme when no theme-color element exists", () => {
    meta?.remove();
    meta = undefined;
    window.matchMedia = () => ({ matches: true });
    const { theme } = runTheme();
    expect(theme).toBe("dark");
    // No theme-color element, so the script leaves the color untouched.
    expect(document.querySelector('meta[name="theme-color"]')).toBeNull();
  });
});
