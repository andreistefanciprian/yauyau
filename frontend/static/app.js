// Drives the single "Add Event" dialog: step 1 picks an event type, step 2
// shows only that type's form. Each form posts straight to its existing
// endpoint (/nappies, /feeds, /pumps, /baths, /sleeps, /observations) via htmx and swaps in
// the refreshed #timeline on success.

const dialog = document.getElementById("add-event-dialog");
const openButton = document.getElementById("add-event-button");
const closeButton = document.getElementById("add-event-close");
const backButton = document.getElementById("add-event-back");
const picker = document.getElementById("event-type-picker");
const title = document.getElementById("add-event-title");
const addEventForms = Array.from(dialog.querySelectorAll(".event-form"));

const typeLabels = {
  nappy: "Log a nappy",
  feed: "Log a feed",
  pump: "Log pumping",
  bath: "Log a bath",
  sleep: "Log sleep",
  observation: "Log an observation",
};

function showPickerStep() {
  picker.hidden = false;
  backButton.hidden = true;
  title.textContent = "Add event";
  addEventForms.forEach((form) => {
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

  const form = addEventForms.find((f) => f.dataset.type === type);
  addEventForms.forEach((f) => {
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

function adjustNumberStepper(button) {
  const stepper = button.closest(".number-stepper");
  if (!stepper) return;

  const input = stepper.querySelector('input[type="number"]');
  if (!input) return;

  const step = Number(button.dataset.stepAmount);
  const current = Number(input.value || input.defaultValue || input.min || 0);
  const min = input.min === "" ? Number.NEGATIVE_INFINITY : Number(input.min);
  const next = Math.max(min, current + step);
  input.value = String(next);
  input.dispatchEvent(new Event("input", { bubbles: true }));
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

document.body.addEventListener("click", (event) => {
  const stepperButton = event.target.closest(".number-stepper-button");
  if (stepperButton) adjustNumberStepper(stepperButton);
});

// Clicking the backdrop (outside the dialog's content box) closes it.
dialog.addEventListener("click", (event) => {
  if (event.target === dialog) dialog.close();
});

// Reset every form once the dialog is dismissed, however that happened
// (close button, backdrop click, Esc, or a successful save).
dialog.addEventListener("close", () => {
  addEventForms.forEach((form) => form.reset());
});

function onEventSaved() {
  dialog.close();
}

window.onEventSaved = onEventSaved;

// Event editing uses one dialog with type-specific sections. Timeline cards
// carry their current values in data-* attributes so the dialog can open
// immediately without another backend request.

const editDialog = document.getElementById("edit-event-dialog");
const editForm = document.getElementById("edit-event-form");
const editCloseButton = document.getElementById("edit-event-close");
const editTypeInput = document.getElementById("edit-event-type");
const editSections = Array.from(document.querySelectorAll(".edit-event-fields"));
const editTitle = document.getElementById("edit-event-title");

function setSectionEnabled(section, enabled) {
  section.hidden = !enabled;
  section.querySelectorAll("input, textarea, select").forEach((field) => {
    field.disabled = !enabled;
  });
}

editSections.forEach((section) => setSectionEnabled(section, false));

function setRadioValue(form, name, value, fallback) {
  const targetValue = value || fallback;
  form.querySelectorAll(`input[type="radio"][name="${name}"]`).forEach((radio) => {
    radio.checked = radio.value === targetValue;
  });
}

function setFieldValue(form, name, value) {
  const field = form.querySelector(`[name="${name}"]`);
  if (field) field.value = value || "";
}

function editSectionForType(type) {
  return editSections.find((section) => section.dataset.editType === type);
}

function openEditDialog(button) {
  const type = button.dataset.eventType;
  const eventID = button.dataset.eventId;
  if (!type || !eventID) return;

  editForm.reset();
  const patchURL = `/events/${eventID}?range=${encodeURIComponent(selectedTimelineRange())}`;
  editForm.setAttribute("hx-patch", patchURL);
  editForm.dataset.patchUrl = patchURL;
  editTypeInput.value = type;
  editTitle.textContent = typeLabels[type] ? typeLabels[type].replace("Log", "Edit") : "Edit event";

  editSections.forEach((section) => {
    setSectionEnabled(section, section.dataset.editType === type);
  });
  const activeSection = editSectionForType(type);
  if (!activeSection) return;

  setFieldValue(editForm, "date", button.dataset.date);
  setFieldValue(editForm, "time", button.dataset.time);

  switch (type) {
    case "nappy":
      setRadioValue(activeSection, "kind", button.dataset.kind, "wet");
      setFieldValue(activeSection, "colour", button.dataset.colour);
      break;
    case "feed":
      setRadioValue(activeSection, "type", button.dataset.type, "breast");
      setFieldValue(activeSection, "amount_ml", button.dataset.amountMl);
      setFieldValue(activeSection, "duration_minutes", button.dataset.durationMinutes);
      break;
    case "pump":
      setFieldValue(activeSection, "amount_ml", button.dataset.amountMl);
      setFieldValue(activeSection, "notes", button.dataset.notes);
      break;
    case "bath":
      setRadioValue(activeSection, "type", button.dataset.type, "whole_body");
      setFieldValue(activeSection, "notes", button.dataset.notes);
      setFieldValue(activeSection, "duration_minutes", button.dataset.durationMinutes);
      break;
    case "sleep":
      setRadioValue(activeSection, "type", button.dataset.type, "nap");
      setFieldValue(activeSection, "notes", button.dataset.notes);
      setFieldValue(activeSection, "duration_minutes", button.dataset.durationMinutes);
      break;
    case "observation":
      setFieldValue(activeSection, "text", button.dataset.text);
      setFieldValue(activeSection, "category", button.dataset.category);
      break;
  }

  editDialog.showModal();
}

function selectedTimelineRange() {
  const rangeInput = editForm.querySelector('input[name="range"]');
  return rangeInput ? rangeInput.value : "today";
}

document.body.addEventListener("click", (event) => {
  const editButton = event.target.closest(".event-edit");
  if (editButton) openEditDialog(editButton);
});

editCloseButton.addEventListener("click", () => editDialog.close());

editDialog.addEventListener("click", (event) => {
  if (event.target === editDialog) editDialog.close();
});

editDialog.addEventListener("close", () => {
  editForm.reset();
  delete editForm.dataset.patchUrl;
  editSections.forEach((section) => setSectionEnabled(section, false));
});

editForm.addEventListener("htmx:configRequest", (event) => {
  if (editForm.dataset.patchUrl) event.detail.path = editForm.dataset.patchUrl;
});

function onEventEdited() {
  editDialog.close();
}

window.onEventEdited = onEventEdited;

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
