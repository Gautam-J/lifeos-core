// The calendar button of a field of a day (#50). The text field holds the
// day as DD-MM-YYYY. A date input would show the day in the format of the
// system, so the page uses one only to open the calendar of the browser, and
// it writes the choice back into the text field in DD-MM-YYYY. The field
// works with no script: Gautam types the day.
(function () {
	function toISO(text) {
		var m = /^(\d\d)-(\d\d)-(\d{4})$/.exec(text.trim());
		return m ? m[3] + "-" + m[2] + "-" + m[1] : "";
	}

	function toDMY(iso) {
		var m = /^(\d{4})-(\d\d)-(\d\d)$/.exec(iso);
		return m ? m[3] + "-" + m[2] + "-" + m[1] : "";
	}

	// native returns the date input of a field. The first click makes it.
	function native(field, text) {
		var picker = field.querySelector(".day-native");
		if (picker) {
			return picker;
		}
		picker = document.createElement("input");
		picker.type = "date";
		picker.className = "day-native";
		picker.tabIndex = -1;
		picker.setAttribute("aria-hidden", "true");
		picker.addEventListener("change", function () {
			text.value = toDMY(picker.value);
			text.dispatchEvent(new Event("input", { bubbles: true }));
			text.focus();
		});
		field.appendChild(picker);
		return picker;
	}

	document.addEventListener("click", function (event) {
		var button = event.target.closest(".day-pick");
		if (!button) {
			return;
		}
		var field = button.closest(".day-field");
		var text = field.querySelector("input.day");
		var picker = native(field, text);
		picker.max = text.getAttribute("data-max") || "";
		picker.value = toISO(text.value);
		try {
			picker.showPicker();
		} catch (e) {
			// A browser without showPicker, or one that refuses it, shows
			// no calendar. The text field still works.
			text.focus();
		}
	});
})();
