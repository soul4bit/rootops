const header = document.querySelector("[data-header]");
const filterButtons = document.querySelectorAll("[data-filter]");
const modules = document.querySelectorAll("[data-stage]");
const navLinks = document.querySelectorAll(".site-nav a[href^='#']");
const navSections = [...navLinks]
  .map((link) => document.querySelector(link.getAttribute("href")))
  .filter(Boolean);

const authModal = document.querySelector("[data-auth-modal]");
const authTitle = document.querySelector("#auth-title");
const authMessage = document.querySelector("[data-auth-message]");
const authTabs = document.querySelectorAll("[data-auth-tab]");
const authForms = document.querySelectorAll("[data-auth-form]");
const authOpeners = document.querySelectorAll("[data-auth-open]");
const authClosers = document.querySelectorAll("[data-auth-close]");
const loginAction = document.querySelector(".login-action");
const signupAction = document.querySelector(".signup-action");

let csrfToken = "";

const updateHeader = () => {
  header?.classList.toggle("is-scrolled", window.scrollY > 16);
};

const updateNavigation = () => {
  if (!navSections.length) return;

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

const setAuthMessage = (message = "", type = "") => {
  if (!authMessage) return;
  authMessage.textContent = message;
  authMessage.classList.toggle("is-error", type === "error");
  authMessage.classList.toggle("is-success", type === "success");
};

const setAuthMode = (mode) => {
  const normalizedMode = mode === "register" ? "register" : "login";

  authTabs.forEach((tab) => {
    tab.classList.toggle("is-active", tab.dataset.authTab === normalizedMode);
  });

  authForms.forEach((form) => {
    form.classList.toggle("is-hidden", form.dataset.authForm !== normalizedMode);
  });

  if (authTitle) {
    authTitle.textContent = normalizedMode === "register" ? "Создай аккаунт" : "Вход в RootOPS";
  }

  setAuthMessage();
};

const loadCsrfToken = async () => {
  if (csrfToken) return csrfToken;

  const response = await fetch("/api/auth/csrf", {
    credentials: "same-origin",
    headers: { Accept: "application/json" },
  });

  if (!response.ok) {
    throw new Error("CSRF token request failed");
  }

  const payload = await response.json();
  csrfToken = payload.csrfToken;
  return csrfToken;
};

const openAuth = async (mode = "login") => {
  if (!authModal) return;

  setAuthMode(mode);
  authModal.hidden = false;
  document.body.classList.add("is-auth-open");

  try {
    await loadCsrfToken();
  } catch {
    setAuthMessage(
      "Для защищённого входа запусти auth-сервер: python server/rootops_auth.py",
      "error",
    );
  }

  const firstInput = authModal.querySelector(`[data-auth-form="${mode}"] input`);
  firstInput?.focus();
};

const closeAuth = () => {
  if (!authModal) return;
  authModal.hidden = true;
  document.body.classList.remove("is-auth-open");
};

const requestJson = async (url, body) => {
  const token = await loadCsrfToken();
  const response = await fetch(url, {
    method: "POST",
    credentials: "same-origin",
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
      "X-CSRF-Token": token,
    },
    body: JSON.stringify({ ...body, csrfToken: token }),
  });

  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(payload.error || "Запрос не выполнен");
  }

  csrfToken = payload.csrfToken || csrfToken;
  return payload;
};

const handleAuthSubmit = async (event) => {
  event.preventDefault();

  const form = event.currentTarget;
  const mode = form.dataset.authForm;
  const formData = new FormData(form);
  const payload = Object.fromEntries(formData.entries());

  setAuthMessage("Проверяем данные...", "success");

  try {
    await requestJson(`/api/auth/${mode}`, payload);
    setAuthMessage("Готово. Открываем кабинет...", "success");
    window.location.assign("/dashboard");
  } catch (error) {
    setAuthMessage(error.message, "error");
  }
};

const logout = async (event) => {
  event.preventDefault();
  try {
    await requestJson("/api/auth/logout", {});
  } finally {
    window.location.assign("/");
  }
};

const refreshAuthState = async () => {
  try {
    const response = await fetch("/api/auth/me", {
      credentials: "same-origin",
      headers: { Accept: "application/json" },
    });

    if (!response.ok) return;
    const payload = await response.json();

    if (payload.authenticated) {
      if (loginAction) {
        loginAction.textContent = "Кабинет";
        loginAction.href = "/dashboard";
        loginAction.removeAttribute("data-auth-open");
      }

      if (signupAction) {
        signupAction.textContent = "Выйти";
        signupAction.href = "/logout";
        signupAction.removeAttribute("data-auth-open");
        signupAction.addEventListener("click", logout);
      }
    }
  } catch {
    // Static preview mode: auth API is simply unavailable.
  }
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
refreshAuthState();

filterButtons.forEach((button) => {
  button.addEventListener("click", () => {
    filterButtons.forEach((item) => {
      item.classList.toggle("is-active", item === button);
      item.setAttribute("aria-pressed", String(item === button));
    });

    applyFilter(button.dataset.filter);
  });
});

authOpeners.forEach((opener) => {
  opener.addEventListener("click", (event) => {
    const mode = opener.dataset.authOpen;
    if (!mode) return;

    event.preventDefault();
    openAuth(mode);
  });
});

authClosers.forEach((closer) => {
  closer.addEventListener("click", closeAuth);
});

authTabs.forEach((tab) => {
  tab.addEventListener("click", () => {
    setAuthMode(tab.dataset.authTab);
  });
});

authForms.forEach((form) => {
  form.addEventListener("submit", handleAuthSubmit);
});

window.addEventListener("keydown", (event) => {
  if (event.key === "Escape") closeAuth();
});

const initialAuthMode = new URLSearchParams(window.location.search).get("auth");
if (initialAuthMode === "login" || initialAuthMode === "register") {
  openAuth(initialAuthMode);
}
