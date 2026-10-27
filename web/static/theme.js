(function () {
  var choice;
  try {
    choice = localStorage.getItem("annalist-theme");
  } catch (_) {}
  document.documentElement.dataset.theme =
    choice === "light" || choice === "dark"
      ? choice
      : window.matchMedia("(prefers-color-scheme: dark)").matches
        ? "dark"
        : "light";
})();
