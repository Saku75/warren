// Warren UI behaviors. Plain script, no build step.

// warrenSlugify mirrors internal/core/slug.Make: transliterate, fold
// diacritics, lowercase, collapse everything else to single hyphens,
// trim, cap at 64. Keep the two in sync — the server re-validates, so a
// drift shows up as a form error rather than bad data.
function warrenSlugify(name) {
	const translit = {
		"æ": "ae", "ø": "o", "å": "a", "ß": "ss", "œ": "oe",
		"đ": "d", "ð": "d", "þ": "th", "ł": "l", "ı": "i",
	};
	let out = "";
	for (const ch of String(name).toLowerCase()) {
		out += translit[ch] ?? ch;
	}
	out = out.normalize("NFKD").replace(/\p{M}+/gu, "");
	out = out.replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "");
	if (out.length > 64) {
		out = out.slice(0, 64).replace(/-+$/g, "");
	}
	return out;
}

// Slug fields follow their name field while in sync, stick once
// customized, and resume following when edited back into sync (or
// cleared). Stateless rule: on every name keystroke the slug updates iff
// it is empty or equals the slug of the name as it was before the
// keystroke. Works unchanged for fresh create forms, pre-filled edit
// forms, and htmx-swapped content.
if (typeof document !== "undefined") {
	document.addEventListener("input", (e) => {
		if (!(e.target instanceof Element) || !e.target.matches("[data-slug-source]")) {
			return;
		}
		const form = e.target.closest("form");
		const slug = form && form.querySelector("[data-slug-target]");
		if (!slug) {
			return;
		}
		const prevName = e.target.dataset.prevName ?? e.target.defaultValue;
		if (slug.value === "" || slug.value === warrenSlugify(prevName)) {
			slug.value = warrenSlugify(e.target.value);
		}
		e.target.dataset.prevName = e.target.value;
	});

	document.addEventListener("click", (e) => {
		if (!(e.target instanceof Element)) {
			return;
		}
		const btn = e.target.closest("[data-slug-regen]");
		if (!btn) {
			return;
		}
		const form = btn.closest("form");
		const name = form && form.querySelector("[data-slug-source]");
		const slug = form && form.querySelector("[data-slug-target]");
		if (name && slug) {
			slug.value = warrenSlugify(name.value);
		}
	});
}
