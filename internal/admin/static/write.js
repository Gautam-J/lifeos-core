// The Write button of the header (#33). It puts the cursor in the text box
// of the journal, so that typing starts at once. On /admin the click does
// it, because Safari on a phone opens the keyboard only for a focus inside
// the handler of a tap. On another page the link opens /admin#write and
// this script focuses the box when the page loads. Without the script the
// link scrolls to the box, as it did before. Focus never clears the text.
(function () {
	function focusBox() {
		var box = document.querySelector("#write-form textarea");
		if (!box) {
			return false;
		}
		box.focus();
		// The cursor goes after the text that is there already.
		box.setSelectionRange(box.value.length, box.value.length);
		box.scrollIntoView({ block: "center" });
		return true;
	}

	document.addEventListener("click", function (event) {
		var link = event.target.closest("[data-write]");
		if (link && document.querySelector("#write-form textarea")) {
			event.preventDefault();
			focusBox();
			history.replaceState(null, "", "#write");
		}
	});

	function onHash() {
		if (location.hash === "#write") {
			focusBox();
		}
	}

	window.addEventListener("hashchange", onHash);
	onHash();
})();
