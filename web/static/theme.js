(function () {
  try {
    var choice;
    try {
      choice = localStorage.getItem("annalist-theme");
    } catch (_) {}
    var resolved =
      choice === "light" || choice === "dark"
        ? choice
        : window.matchMedia("(prefers-color-scheme: dark)").matches
          ? "dark"
          : "light";
    document.documentElement.dataset.theme = resolved;
    var meta = document.querySelector('meta[name="theme-color"]');
    if (meta) meta.content = resolved === "dark" ? "#211d19" : "#f5eee5";
  } catch (_) {
    // No matchMedia/storage: fall back to light rather than ship an unthemed
    // page.
    document.documentElement.dataset.theme = "light";
  }
})();
