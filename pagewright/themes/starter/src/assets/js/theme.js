(function () {
  const btn = document.querySelector("[data-nav-toggle]");
  const nav = document.querySelector("[data-nav]");
  if (!btn || !nav) return;

  btn.addEventListener("click", () => {
    const open = document.body.classList.toggle("nav-open");
    btn.setAttribute("aria-expanded", open ? "true" : "false");
  });
})();
