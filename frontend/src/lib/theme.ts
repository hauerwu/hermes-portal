/** 主题管理：dark（默认）/ light，持久化到 localStorage。 */
export type Theme = "dark" | "light";

const THEME_KEY = "portal.theme";

export function getTheme(): Theme {
  return localStorage.getItem(THEME_KEY) === "light" ? "light" : "dark";
}

export function applyTheme(theme: Theme) {
  document.documentElement.setAttribute("data-theme", theme);
  localStorage.setItem(THEME_KEY, theme);
}

/** 应用初始主题（在 React 挂载前调用，避免闪烁）。 */
export function initTheme() {
  applyTheme(getTheme());
}

export function toggleTheme(): Theme {
  const next: Theme = getTheme() === "dark" ? "light" : "dark";
  applyTheme(next);
  return next;
}
