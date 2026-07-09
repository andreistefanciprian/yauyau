// Drives the single "Add Event" dialog: step 1 picks an event type, step 2
// shows only that type's form. Each form posts straight to its existing
// endpoint (/nappies, /feeds, /baths, /sleeps, /observations) via htmx and swaps in
// the refreshed #timeline on success.

const dialog = document.getElementById("add-event-dialog");
const openButton = document.getElementById("add-event-button");
const closeButton = document.getElementById("add-event-close");
const backButton = document.getElementById("add-event-back");
const picker = document.getElementById("event-type-picker");
const title = document.getElementById("add-event-title");
const forms = Array.from(document.querySelectorAll(".event-form"));

const typeLabels = {
  nappy: "Log a nappy",
  feed: "Log a feed",
  bath: "Log a bath",
  sleep: "Log sleep",
  observation: "Log an observation",
};

function showPickerStep() {
  picker.hidden = false;
  backButton.hidden = true;
  title.textContent = "Add event";
  forms.forEach((form) => {
    form.hidden = true;
    // Clear whatever was entered so going Back and picking a type again
    // (the same one or a different one) never resubmits stale field
    // values or a manually-backdated time the user meant to keep editing.
    form.reset();
  });
}

function showFormStep(type) {
  picker.hidden = true;
  backButton.hidden = false;
  title.textContent = typeLabels[type] || "Add event";

  const form = forms.find((f) => f.dataset.type === type);
  forms.forEach((f) => {
    f.hidden = f !== form;
  });
  if (form) {
    setFormToNow(form);
    const firstField = form.querySelector("input, textarea");
    if (firstField) firstField.focus();
  }
}

// Set a form's date/time fields to the current local date/time. Called each
// time a form is shown, since a value baked in at page load would go stale
// if the dialog is opened later in the same session.
function setFormToNow(form) {
  const now = new Date();
  const pad = (n) => String(n).padStart(2, "0");

  const dateInput = form.querySelector('input[type="date"]');
  if (dateInput) {
    dateInput.value = `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`;
  }

  const timeInput = form.querySelector('input[type="time"]');
  if (timeInput) {
    timeInput.value = `${pad(now.getHours())}:${pad(now.getMinutes())}`;
  }
}

function openDialog() {
  showPickerStep();
  dialog.showModal();
}

openButton.addEventListener("click", openDialog);
closeButton.addEventListener("click", () => dialog.close());
backButton.addEventListener("click", showPickerStep);

picker.addEventListener("click", (event) => {
  const choice = event.target.closest(".type-choice");
  if (choice) showFormStep(choice.dataset.type);
});

// Clicking the backdrop (outside the dialog's content box) closes it.
dialog.addEventListener("click", (event) => {
  if (event.target === dialog) dialog.close();
});

// Reset every form once the dialog is dismissed, however that happened
// (close button, backdrop click, Esc, or a successful save).
dialog.addEventListener("close", () => {
  forms.forEach((form) => form.reset());
});

function onEventSaved() {
  dialog.close();
}

window.onEventSaved = onEventSaved;

// Replaces the native window.confirm() that htmx's hx-confirm would
// otherwise trigger (e.g. for event deletion) with a styled dialog, since
// window.confirm() can't be themed at all.

const confirmDialog = document.getElementById("confirm-dialog");
const confirmMessage = document.getElementById("confirm-dialog-message");
const confirmCancelButton = document.getElementById("confirm-dialog-cancel");
const confirmAcceptButton = document.getElementById("confirm-dialog-accept");

document.body.addEventListener("htmx:confirm", (event) => {
  if (!event.detail.elt.hasAttribute("hx-confirm")) return;
  event.preventDefault();

  confirmDialog.returnValue = "";
  confirmMessage.textContent = event.detail.question;
  confirmDialog.showModal();

  const onAccept = () => confirmDialog.close("accept");

  const onClose = () => {
    confirmAcceptButton.removeEventListener("click", onAccept);
    if (confirmDialog.returnValue === "accept") {
      event.detail.issueRequest(true);
    }
  };

  confirmAcceptButton.addEventListener("click", onAccept);
  confirmDialog.addEventListener("close", onClose, { once: true });
});

confirmCancelButton.addEventListener("click", () => confirmDialog.close("cancel"));

// Clicking the backdrop (outside the dialog's content box) cancels it.
confirmDialog.addEventListener("click", (event) => {
  if (event.target === confirmDialog) confirmDialog.close("cancel");
});
