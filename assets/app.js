const header = document.querySelector("[data-header]");
const filterButtons = document.querySelectorAll("[data-filter]");
const modules = document.querySelectorAll("[data-stage]");

const updateHeader = () => {
  header?.classList.toggle("is-scrolled", window.scrollY > 16);
};

const applyFilter = (stage) => {
  modules.forEach((module) => {
    const isVisible = stage === "all" || module.dataset.stage === stage;
    module.classList.toggle("is-hidden", !isVisible);
  });
};

window.addEventListener("scroll", updateHeader, { passive: true });
updateHeader();

filterButtons.forEach((button) => {
  button.addEventListener("click", () => {
    filterButtons.forEach((item) => {
      item.classList.toggle("is-active", item === button);
      item.setAttribute("aria-pressed", String(item === button));
    });

    applyFilter(button.dataset.filter);
  });
});
