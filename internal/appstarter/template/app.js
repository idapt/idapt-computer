// __APP_NAME__ — a portable idapt browser-app.
// Runs hosted (cookie SDK), on `idapt app run` localhost, or standalone.
(function () {
  "use strict";
  var statusEl = document.getElementById("status");
  var noteEl = document.getElementById("note");
  var saveEl = document.getElementById("save");
  var savedEl = document.getElementById("saved");

  function setStatus(text) {
    statusEl.textContent = text;
  }

  // The SDK exposes window.Idapt only when served by idapt; degrade gracefully.
  if (!window.Idapt) {
    setStatus("Running standalone (no idapt connection).");
    saveEl.addEventListener("click", function () {
      try {
        localStorage.setItem("note", noteEl.value);
        savedEl.textContent = "Saved locally.";
      } catch (e) {
        savedEl.textContent = "Could not save.";
      }
    });
    try {
      noteEl.value = localStorage.getItem("note") || "";
    } catch (e) {}
    return;
  }

  window.Idapt.connect()
    .then(function (client) {
      setStatus("Connected.");
      return client.data
        .getJSON("note")
        .catch(function () {
          return null;
        })
        .then(function (stored) {
          if (stored && typeof stored.text === "string") {
            noteEl.value = stored.text;
          }
          saveEl.addEventListener("click", function () {
            savedEl.textContent = "Saving…";
            client.data
              .setJSON("note", { text: noteEl.value })
              .then(function () {
                savedEl.textContent = "Saved.";
              })
              .catch(function () {
                savedEl.textContent = "Save failed.";
              });
          });
        });
    })
    .catch(function () {
      setStatus("Could not connect to idapt.");
    });
})();
