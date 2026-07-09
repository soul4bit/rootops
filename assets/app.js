const header = document.querySelector("[data-header]");
const filterButtons = document.querySelectorAll("[data-filter]");
const modules = document.querySelectorAll("[data-stage]");
const navLinks = document.querySelectorAll(".site-nav a[href^='#']");
const navSections = [...navLinks]
  .map((link) => document.querySelector(link.getAttribute("href")))
  .filter(Boolean);

const updateHeader = () => {
  header?.classList.toggle("is-scrolled", window.scrollY > 16);
};

const updateNavigation = () => {
  const current = navSections.reduce((active, section) => {
    const sectionTop = section.getBoundingClientRect().top;
    return sectionTop <= 120 ? section : active;
  }, navSections[0]);

  navLinks.forEach((link) => {
    link.classList.toggle("is-active", link.getAttribute("href") === `#${current?.id}`);
  });
};

const applyFilter = (stage) => {
  modules.forEach((module) => {
    const isVisible = stage === "all" || module.dataset.stage === stage;
    module.classList.toggle("is-hidden", !isVisible);
  });
};

window.addEventListener(
  "scroll",
  () => {
    updateHeader();
    updateNavigation();
  },
  { passive: true },
);
updateHeader();
updateNavigation();

filterButtons.forEach((button) => {
  button.addEventListener("click", () => {
    filterButtons.forEach((item) => {
      item.classList.toggle("is-active", item === button);
      item.setAttribute("aria-pressed", String(item === button));
    });

    applyFilter(button.dataset.filter);
  });
});
