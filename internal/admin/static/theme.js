// The theme of the admin pages. With no choice, the pages follow the
// setting of the system. A choice of Light or Dark stays in localStorage of
// this browser. The server stores nothing.
// This file runs before the first paint, so that a page does not show the
// wrong theme first.
(function () {
	var root = document.documentElement;

	function stored() {
		try {
			var theme = localStorage.getItem("theme");
			return theme === "light" || theme === "dark" ? theme : "system";
		} catch (e) {
			// A browser can refuse the storage. The pages then follow the system.
			return "system";
		}
	}

	function apply(theme) {
		if (theme === "system") {
			root.removeAttribute("data-theme");
		} else {
			root.setAttribute("data-theme", theme);
		}
	}

	// paint gives the bar of the browser the color of the page.
	function paint() {
		var canvas = getComputedStyle(root).getPropertyValue("--canvas").trim();
		var meta = document.querySelector('meta[name="theme-color"]');
		if (meta && canvas) {
			meta.setAttribute("content", canvas);
		}
	}

	// mark shows which button is the current choice.
	function mark() {
		var theme = stored();
		document.querySelectorAll("[data-theme-set]").forEach(function (button) {
			button.setAttribute("aria-checked", String(button.getAttribute("data-theme-set") === theme));
		});
		paint();
	}

	apply(stored());

	document.addEventListener("DOMContentLoaded", mark);
	document.addEventListener("click", function (event) {
		var button = event.target.closest("[data-theme-set]");
		if (!button) {
			return;
		}
		var theme = button.getAttribute("data-theme-set");
		try {
			if (theme === "system") {
				localStorage.removeItem("theme");
			} else {
				localStorage.setItem("theme", theme);
			}
		} catch (e) {
			// The choice then holds for this page only.
		}
		apply(theme);
		mark();
	});
	// The system changes between light and dark, for example at sunset.
	window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", paint);
	// A second tab made a choice.
	window.addEventListener("storage", function () {
		apply(stored());
		mark();
	});
})();
