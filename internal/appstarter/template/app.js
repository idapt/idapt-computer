// __APP_NAME__ — a portable idapt browser-app (no build step).
// Hosted on an idapt app subdomain it persists to your private app-data KV via
// the app-key cookie; opened standalone (file://) it falls back to localStorage.
(function () {
  "use strict";
  var statusEl = document.getElementById("status");
  var noteEl = document.getElementById("note");
  var saveEl = document.getElementById("save");
  var savedEl = document.getElementById("saved");

  function setStatus(text) {
    statusEl.textContent = text;
  }

  // The app id is in the server-injected #__idapt-boot config blob; absent when
  // opened off idapt (static host / file://).
  function appId() {
    try {
      var el = document.getElementById("__idapt-boot");
      return el ? (JSON.parse(el.textContent || "{}").appId || "") : "";
    } catch (e) {
      return "";
    }
  }

  var id = appId();
  if (!id) {
    // Off idapt — degrade to localStorage so the app still works standalone.
    setStatus("Running standalone (no idapt connection).");
    try {
      noteEl.value = localStorage.getItem("note") || "";
    } catch (e) {}
    saveEl.addEventListener("click", function () {
      try {
        localStorage.setItem("note", noteEl.value);
        savedEl.textContent = "Saved locally.";
      } catch (e) {
        savedEl.textContent = "Could not save.";
      }
    });
    return;
  }

  // Hosted on idapt: read/write the per-app KV with the app-key cookie.
  var base = "/api/browser-app/data/" + encodeURIComponent(id) + "/note";
  setStatus("Connected.");
  fetch(base, { credentials: "include" })
    .then(function (r) {
      return r.ok ? r.json() : null;
    })
    .then(function (body) {
      var stored = body && "value" in body ? body.value : null;
      if (stored && typeof stored.text === "string") {
        noteEl.value = stored.text;
      }
    })
    .catch(function () {});
  saveEl.addEventListener("click", function () {
    savedEl.textContent = "Saving…";
    fetch(base, {
      method: "PUT",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ value: { text: noteEl.value } }),
    })
      .then(function (r) {
        savedEl.textContent = r.ok ? "Saved." : "Save failed.";
      })
      .catch(function () {
        savedEl.textContent = "Save failed.";
      });
  });
})();
