// Disable htmx's history snapshot/restore. By default htmx caches a
// full-page HTML snapshot on every hx-push-url and, on browser back/forward,
// either replaces the whole document with that stale snapshot (orphaning
// every top-level DOM reference below - dialog, filtersToggle, typeFilter -
// and breaking the add-event dialog, filters, everything) or, on a cache
// miss, re-requests the current URL itself and dumps the (deliberately
// partial, #timeline-workspace-only) response straight into <body> -
// wiping out the navbar and sidebar, since that recovery path swaps into
// <body> rather than the original click's hx-target. historyCacheSize: 0
// guarantees every back/forward is a "miss"; refreshOnHistoryMiss routes
// that miss to a real full-page reload instead of htmx's own broken
// recovery - i.e. back/forward behaves exactly like it did before this
// file added instant day-switching, while a direct pill click still is.
htmx.config.historyCacheSize = 0;
htmx.config.refreshOnHistoryMiss = true;

// Drives the single "Add Event" dialog: step 1 picks an event type, step 2
// shows only that type's form. Each form posts straight to its existing
// endpoint (/nappies, /feeds, /pumps, /baths, /sleeps, /observations, /temperatures, /growth-measurements) via htmx and swaps in
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
  temperature: "Log temperature",
  growth_measurement: "Log growth",
};

function hideDialogError(dialogEl) {
  const errorEl = dialogEl.querySelector(".dialog-error");
  if (errorEl) errorEl.hidden = true;
}

// A save/edit that fails (e.g. backend-api rejecting a future-dated event)
// gets its message shown here instead of failing silently — htmx doesn't
// swap non-2xx responses into the page by default, so without this the
// dialog would just sit there with no indication anything went wrong.
document.body.addEventListener("htmx:responseError", (event) => {
  const dialogEl = event.target.closest("dialog");
  if (!dialogEl) return;
  const errorEl = dialogEl.querySelector(".dialog-error");
  if (!errorEl) return;
  errorEl.textContent = event.detail.xhr.responseText || "Something went wrong. Please try again.";
  errorEl.hidden = false;
});

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

function localDateValue(date) {
  const pad = (n) => String(n).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

function localTimeValue(date) {
  const pad = (n) => String(n).padStart(2, "0");
  return `${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

function parseLocalDateTime(dateValue, timeValue) {
  if (!dateValue || !timeValue) return null;
  const [year, month, day] = dateValue.split("-").map(Number);
  const [hour, minute] = timeValue.split(":").map(Number);
  if (![year, month, day, hour, minute].every(Number.isFinite)) return null;
  return new Date(year, month - 1, day, hour, minute);
}

function formatDuration(minutes) {
  if (!Number.isFinite(minutes) || minutes <= 0) return "";
  const hours = Math.floor(minutes / 60);
  const remainingMinutes = minutes % 60;
  if (hours === 0) return `${remainingMinutes} min`;
  if (remainingMinutes === 0) return `${hours}h`;
  return `${hours}h ${remainingMinutes}m`;
}

function selectedRadioValue(scope, name) {
  return scope.querySelector(`input[type="radio"][name="${name}"]:checked`)?.value || "";
}

function updatePooSizeFields(scope) {
  const containers = scope.querySelectorAll("[data-poo-size-field]");
  containers.forEach((container) => {
    const form = container.closest("form");
    if (!form) return;

    const kind = selectedRadioValue(form, "kind");
    const show = kind === "poo" || kind === "both";
    container.disabled = !show;
  });
}

function updateFeedAmountFields(scope) {
  scope.querySelectorAll("[data-feed-amount-field]").forEach((container) => {
    const feedFields = container.closest('form[data-type="feed"], [data-edit-type="feed"]');
    if (!feedFields || feedFields.hidden) return;

    const feedType = selectedRadioValue(feedFields, "type");
    const disableAmount = feedType === "breast";
    container.classList.toggle("field-disabled", disableAmount);
    container.querySelectorAll("input").forEach((input) => {
      input.disabled = disableAmount;
      if (disableAmount) input.value = "";
    });
  });
}

// Set a form's date/time fields to the current local date/time. Called each
// time a form is shown, since a value baked in at page load would go stale
// if the dialog is opened later in the same session.
function setFormToNow(form) {
  const now = new Date();

  const dateInput = form.querySelector('input[type="date"]');
  if (dateInput) {
    dateInput.value = localDateValue(now);
    dateInput.max = localDateValue(now);
  }

  const timeInput = form.querySelector('input[type="time"]');
  if (timeInput) {
    timeInput.value = localTimeValue(now);
  }

  form.querySelectorAll("[data-sleep-end-date]").forEach((input) => {
    input.value = "";
    input.max = localDateValue(now);
  });
  form.querySelectorAll("[data-sleep-end-time]").forEach((input) => {
    input.value = "";
  });
  form.querySelectorAll("[data-feed-end-date]").forEach((input) => {
    input.value = "";
    input.max = localDateValue(now);
  });
  form.querySelectorAll("[data-feed-end-time]").forEach((input) => {
    input.value = "";
  });
  form.querySelectorAll("[data-pump-end-date]").forEach((input) => {
    input.value = "";
    input.max = localDateValue(now);
  });
  form.querySelectorAll("[data-pump-end-time]").forEach((input) => {
    input.value = "";
  });
  updateSleepDuration(form);
  updateFeedDuration(form);
  updatePumpDuration(form);
  updatePooSizeFields(form);
  updateFeedAmountFields(form);
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
  addEventForms.forEach((form) => form.reset());
  hideDialogError(dialog);
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
const editDeleteButton = document.getElementById("edit-event-delete");
const editSaveButton = document.getElementById("edit-event-save");
const editTypeInput = document.getElementById("edit-event-type");
const editSections = Array.from(document.querySelectorAll(".edit-event-fields"));
const editTitle = document.getElementById("edit-event-title");
const editTimeLabel = editForm.querySelector("[data-edit-time-label]");
const editDateLabel = editForm.querySelector("[data-edit-date-label]");
const editOccurredAtFields = editForm.querySelector("[data-edit-occurred-at-fields]");
const editOccurredAtLabel = editForm.querySelector("[data-edit-occurred-at-label]");
let editFormBaseline = "";

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
  if (field) {
    field.value = value || "";
    field.dispatchEvent(new Event("input", { bubbles: true }));
  }
}

function setCheckboxValues(form, name, rawValues) {
  const values = new Set((rawValues || "").split(",").filter(Boolean));
  form.querySelectorAll(`input[type="checkbox"][name="${name}"]`).forEach((checkbox) => {
    checkbox.checked = values.has(checkbox.value);
  });
}

function setSleepEndFromStart(form, durationMinutes) {
  const minutes = Number.parseInt(durationMinutes, 10);
  const startDate = form.querySelector('input[name="date"]');
  const startTime = form.querySelector('input[name="time"]');
  const endDate = form.querySelector("[data-sleep-end-date]");
  const endTime = form.querySelector("[data-sleep-end-time]");
  const start = parseLocalDateTime(startDate?.value, startTime?.value);
  if (!start || !endDate || !endTime) return;

  if (Number.isFinite(minutes) && minutes > 0) {
    const end = new Date(start);
    end.setMinutes(end.getMinutes() + minutes);
    endDate.value = localDateValue(end);
    endTime.value = localTimeValue(end);
    return;
  }

  endDate.value = "";
  endTime.value = "";
}

function setFeedEndFromStart(form, durationMinutes) {
  const minutes = Number.parseInt(durationMinutes, 10);
  const startDate = form.querySelector('input[name="date"]');
  const startTime = form.querySelector('input[name="time"]');
  const endDate = form.querySelector("[data-feed-end-date]");
  const endTime = form.querySelector("[data-feed-end-time]");
  const start = parseLocalDateTime(startDate?.value, startTime?.value);
  if (!start || !endDate || !endTime) return;

  if (Number.isFinite(minutes) && minutes > 0) {
    const end = new Date(start);
    end.setMinutes(end.getMinutes() + minutes);
    endDate.value = localDateValue(end);
    endTime.value = localTimeValue(end);
    return;
  }

  endDate.value = "";
  endTime.value = "";
}

function setPumpEndFromStart(form, durationMinutes) {
  const minutes = Number.parseInt(durationMinutes, 10);
  const startDate = form.querySelector('input[name="date"]');
  const startTime = form.querySelector('input[name="time"]');
  const endDate = form.querySelector("[data-pump-end-date]");
  const endTime = form.querySelector("[data-pump-end-time]");
  const start = parseLocalDateTime(startDate?.value, startTime?.value);
  if (!start || !endDate || !endTime) return;

  if (Number.isFinite(minutes) && minutes > 0) {
    const end = new Date(start);
    end.setMinutes(end.getMinutes() + minutes);
    endDate.value = localDateValue(end);
    endTime.value = localTimeValue(end);
    return;
  }

  endDate.value = "";
  endTime.value = "";
}

function syncNumberSliderThumb(input) {
  const slider = input.closest(".number-slider");
  const range = slider?.querySelector(".number-slider-range");
  if (!range || input.value === "") return;

  const value = Number(input.value);
  if (Number.isNaN(value)) return;
  const min = Number(range.min);
  const max = Number(range.max);
  range.value = String(Math.min(max, Math.max(min, value)));
}

function setSleepDurationValue(form, value) {
  const durationInput = form.querySelector("[data-sleep-duration-minutes]");
  if (!durationInput) return;
  durationInput.value = value;
  syncNumberSliderThumb(durationInput);
}

function setFeedDurationValue(form, value) {
  const durationInput = form.querySelector("[data-feed-duration-minutes]");
  if (!durationInput) return;
  durationInput.value = value;
  syncNumberSliderThumb(durationInput);
}

function setPumpDurationValue(form, value) {
  const durationInput = form.querySelector("[data-pump-duration-minutes]");
  if (!durationInput) return;
  durationInput.value = value;
  syncNumberSliderThumb(durationInput);
}

function updateSleepSubmitLabel(form, hasCompletedSleep) {
  const submitButton = form.querySelector("[data-sleep-submit-label]");
  if (!submitButton) return;
  submitButton.textContent = hasCompletedSleep ? "Save" : "Start";
}

function updateFeedSubmitLabel(form, hasCompletedFeed) {
  const submitButton = form.querySelector("[data-feed-submit-label]");
  if (!submitButton) return;
  submitButton.textContent = hasCompletedFeed ? "Save" : "Start";
}

function updatePumpSubmitLabel(form, hasCompletedPump) {
  const submitButton = form.querySelector("[data-pump-submit-label]");
  if (!submitButton) return;
  submitButton.textContent = hasCompletedPump ? "Save" : "Start";
}

function updateSleepDuration(scope) {
  const fields = scope.querySelectorAll("[data-sleep-time-fields]");
  fields.forEach((container) => {
    const form = container.closest("form");
    if (!form) return;

    const startDate = form.querySelector('input[name="date"]');
    const startTime = form.querySelector('input[name="time"]');
    const endDate = container.querySelector("[data-sleep-end-date]");
    const endTime = container.querySelector("[data-sleep-end-time]");
    const preview = container.querySelector("[data-sleep-duration-preview]");
    const start = parseLocalDateTime(startDate?.value, startTime?.value);
    const end = parseLocalDateTime(endDate?.value, endTime?.value);

    endDate.setCustomValidity("");
    endTime.setCustomValidity("");

    if (!start || !end) {
      setSleepDurationValue(form, "");
      updateSleepSubmitLabel(form, false);
      if (preview) preview.textContent = "Add wake-up time to calculate duration.";
      return;
    }

    if (end <= start) {
      const message = "Wake-up time must be after sleep start.";
      endTime.setCustomValidity(message);
      setSleepDurationValue(form, "");
      updateSleepSubmitLabel(form, false);
      if (preview) preview.textContent = message;
      return;
    }

    const minutes = Math.round((end - start) / 60000);
    setSleepDurationValue(form, String(minutes));
    updateSleepSubmitLabel(form, true);
    if (preview) preview.textContent = `Duration: ${formatDuration(minutes)}`;
  });
}

function updateFeedDuration(scope) {
  const fields = scope.querySelectorAll("[data-feed-time-fields]");
  fields.forEach((container) => {
    const form = container.closest("form");
    if (!form) return;

    const startDate = form.querySelector('input[name="date"]');
    const startTime = form.querySelector('input[name="time"]');
    const endDate = container.querySelector("[data-feed-end-date]");
    const endTime = container.querySelector("[data-feed-end-time]");
    const preview = container.querySelector("[data-feed-duration-preview]");
    const start = parseLocalDateTime(startDate?.value, startTime?.value);
    const end = parseLocalDateTime(endDate?.value, endTime?.value);

    endDate.setCustomValidity("");
    endTime.setCustomValidity("");

    if (!start || !end) {
      setFeedDurationValue(form, "");
      updateFeedSubmitLabel(form, false);
      if (preview) preview.textContent = "Add finish time to calculate duration.";
      return;
    }

    if (end <= start) {
      const message = "Finish time must be after feed start.";
      endTime.setCustomValidity(message);
      setFeedDurationValue(form, "");
      updateFeedSubmitLabel(form, false);
      if (preview) preview.textContent = message;
      return;
    }

    const minutes = Math.round((end - start) / 60000);
    setFeedDurationValue(form, String(minutes));
    updateFeedSubmitLabel(form, true);
    if (preview) preview.textContent = `Duration: ${formatDuration(minutes)}`;
  });
}

function updatePumpDuration(scope) {
  const fields = scope.querySelectorAll("[data-pump-time-fields]");
  fields.forEach((container) => {
    const form = container.closest("form");
    if (!form) return;

    const startDate = form.querySelector('input[name="date"]');
    const startTime = form.querySelector('input[name="time"]');
    const endDate = container.querySelector("[data-pump-end-date]");
    const endTime = container.querySelector("[data-pump-end-time]");
    const preview = container.querySelector("[data-pump-duration-preview]");
    const start = parseLocalDateTime(startDate?.value, startTime?.value);
    const end = parseLocalDateTime(endDate?.value, endTime?.value);

    endDate.setCustomValidity("");
    endTime.setCustomValidity("");

    if (!start || !end) {
      setPumpDurationValue(form, "");
      updatePumpSubmitLabel(form, false);
      if (preview) preview.textContent = "Add finish time to calculate duration.";
      return;
    }

    if (end <= start) {
      const message = "Finish time must be after pump start.";
      endTime.setCustomValidity(message);
      setPumpDurationValue(form, "");
      updatePumpSubmitLabel(form, false);
      if (preview) preview.textContent = message;
      return;
    }

    const minutes = Math.round((end - start) / 60000);
    setPumpDurationValue(form, String(minutes));
    updatePumpSubmitLabel(form, true);
    if (preview) preview.textContent = `Duration: ${formatDuration(minutes)}`;
  });
}

function updateSleepEndFromDuration(form) {
  const durationInput = form.querySelector("[data-sleep-duration-minutes]");
  if (!durationInput) return;

  setSleepEndFromStart(form, durationInput.value);
  updateSleepDuration(form);
}

function updateFeedEndFromDuration(form) {
  const durationInput = form.querySelector("[data-feed-duration-minutes]");
  if (!durationInput) return;

  setFeedEndFromStart(form, durationInput.value);
  updateFeedDuration(form);
}

function updatePumpEndFromDuration(form) {
  const durationInput = form.querySelector("[data-pump-duration-minutes]");
  if (!durationInput) return;

  setPumpEndFromStart(form, durationInput.value);
  updatePumpDuration(form);
}

function editSectionForType(type) {
  return editSections.find((section) => section.dataset.editType === type);
}

function editFormState() {
  return JSON.stringify(Array.from(new FormData(editForm).entries()));
}

function updateEditSaveState() {
  editSaveButton.disabled = editFormState() === editFormBaseline;
}

function queueEditSaveStateUpdate() {
  queueMicrotask(updateEditSaveState);
}

function openEditDialog(card) {
  const type = card.dataset.eventType;
  const eventID = card.dataset.eventId;
  if (!type || !eventID) return;

  editForm.reset();
  const patchURL = `/events/${eventID}?selected_date=${encodeURIComponent(selectedTimelineDate())}`;
  editForm.setAttribute("hx-patch", patchURL);
  editForm.dataset.patchUrl = patchURL;
  const deleteURL = `/events/${eventID}?selected_date=${encodeURIComponent(selectedTimelineDate())}`;
  editDeleteButton.setAttribute("hx-delete", deleteURL);
  editDeleteButton.dataset.deleteUrl = deleteURL;
  editTypeInput.value = type;
  editTitle.textContent = typeLabels[type] ? typeLabels[type].replace("Log", "Edit") : "Edit event";
  const groupedStartFields = type === "sleep" || type === "feed" || type === "pump";
  if (editOccurredAtFields) editOccurredAtFields.classList.toggle("grouped-edit-time-fields", groupedStartFields);
  if (editOccurredAtLabel) {
    editOccurredAtLabel.hidden = !groupedStartFields;
    editOccurredAtLabel.textContent = type === "sleep" ? "Fell asleep" : "Started";
  }
  if (editTimeLabel) editTimeLabel.textContent = "Time";
  if (editDateLabel) editDateLabel.textContent = "Date";

  editSections.forEach((section) => {
    setSectionEnabled(section, section.dataset.editType === type);
  });
  const activeSection = editSectionForType(type);
  if (!activeSection) return;

  const editDateInput = editForm.querySelector('input[type="date"]');
  if (editDateInput) editDateInput.max = localDateValue(new Date());

  setFieldValue(editForm, "date", card.dataset.date);
  setFieldValue(editForm, "time", card.dataset.time);

  switch (type) {
    case "nappy":
      setRadioValue(activeSection, "kind", card.dataset.kind, "wet");
      setRadioValue(activeSection, "poo_size", card.dataset.pooSize, "medium");
      setCheckboxValues(activeSection, "labels", card.dataset.labels);
      updatePooSizeFields(editForm);
      setFieldValue(activeSection, "notes", card.dataset.notes);
      break;
    case "feed":
      setRadioValue(activeSection, "type", card.dataset.type, "expressed");
      setFieldValue(activeSection, "amount_ml", card.dataset.amountMl);
      setFieldValue(activeSection, "duration_minutes", card.dataset.durationMinutes);
      setFeedEndFromStart(editForm, card.dataset.durationMinutes);
      updateFeedDuration(editForm);
      updateFeedAmountFields(editForm);
      setCheckboxValues(activeSection, "labels", card.dataset.labels);
      setFieldValue(activeSection, "notes", card.dataset.notes);
      break;
    case "pump":
      setFieldValue(activeSection, "amount_ml", card.dataset.amountMl);
      setFieldValue(activeSection, "notes", card.dataset.notes);
      setFieldValue(activeSection, "duration_minutes", card.dataset.durationMinutes);
      setPumpEndFromStart(editForm, card.dataset.durationMinutes);
      updatePumpDuration(editForm);
      break;
    case "bath":
      setRadioValue(activeSection, "type", card.dataset.type, "bottom_part");
      setRadioValue(activeSection, "bath_time_basis", "start", "start");
      setFieldValue(activeSection, "notes", card.dataset.notes);
      setFieldValue(activeSection, "duration_minutes", card.dataset.durationMinutes);
      break;
    case "sleep":
      setRadioValue(activeSection, "type", card.dataset.type, "nap");
      setFieldValue(activeSection, "notes", card.dataset.notes);
      setFieldValue(activeSection, "duration_minutes", card.dataset.durationMinutes);
      setSleepEndFromStart(editForm, card.dataset.durationMinutes);
      updateSleepDuration(editForm);
      break;
    case "observation":
      setFieldValue(activeSection, "text", card.dataset.text);
      setFieldValue(activeSection, "category", card.dataset.category);
      break;
    case "temperature":
      setFieldValue(activeSection, "temperature_c", card.dataset.temperatureC);
      setFieldValue(activeSection, "method", card.dataset.method || "ear");
      setFieldValue(activeSection, "notes", card.dataset.notes);
      break;
    case "growth_measurement":
      setFieldValue(activeSection, "weight_kg", card.dataset.weightKg);
      setFieldValue(activeSection, "length_cm", card.dataset.lengthCm);
      setFieldValue(activeSection, "head_circumference_cm", card.dataset.headCircumferenceCm);
      setFieldValue(activeSection, "notes", card.dataset.notes);
      break;
  }

  editFormBaseline = editFormState();
  updateEditSaveState();
  editDialog.showModal();
}

function selectedTimelineDate() {
  const dateInput = editForm.querySelector('input[name="selected_date"]');
  return dateInput ? dateInput.value : "";
}

// A quick-view popup shown on row click, ahead of the full edit form: icon,
// title, time, detail, and Edit/Delete actions. Its content is cloned
// straight from the clicked row's own already-rendered markup rather than
// rebuilt from data-* attributes, so it can never drift out of sync with
// how the row itself formats a title/detail/time.
const quickViewDialog = document.getElementById("event-quickview-dialog");
const quickViewMarkerWrap = document.getElementById("quickview-marker-wrap");
const quickViewTitle = document.getElementById("quickview-title");
const quickViewTime = document.getElementById("quickview-time");
const quickViewDetail = document.getElementById("quickview-detail");
const quickViewEditButton = document.getElementById("quickview-edit-button");
const quickViewDeleteButton = document.getElementById("quickview-delete-button");
let quickViewCard = null;

function eventTimeText(card) {
  const clock = card.querySelector(".event-time-clock");
  const date = card.querySelector(".event-time-date");
  const clockText = clock ? clock.textContent.trim() : card.querySelector(".event-time")?.textContent.trim() ?? "";
  return date ? `${date.textContent.trim()}, ${clockText}` : clockText;
}

function openQuickView(card) {
  const eventID = card.dataset.eventId;
  if (!eventID) return;

  quickViewCard = card;

  const cssClass = card.dataset.cssClass;
  const icon = card.querySelector(".event-marker .event-type-icon");
  quickViewMarkerWrap.className = "quickview-marker-wrap" + (cssClass ? ` event-${cssClass}` : "");
  quickViewMarkerWrap.innerHTML = `<span class="event-marker">${icon ? icon.outerHTML : ""}</span>`;

  quickViewTitle.innerHTML = card.querySelector(".event-type")?.innerHTML ?? "";
  quickViewTime.textContent = eventTimeText(card);

  const detail = card.querySelector(".event-detail");
  if (detail) {
    quickViewDetail.innerHTML = detail.innerHTML;
    quickViewDetail.hidden = false;
  } else {
    quickViewDetail.innerHTML = "";
    quickViewDetail.hidden = true;
  }

  const deleteURL = `/events/${eventID}?selected_date=${encodeURIComponent(selectedTimelineDate())}`;
  quickViewDeleteButton.setAttribute("hx-delete", deleteURL);
  quickViewDeleteButton.dataset.deleteUrl = deleteURL;

  quickViewDialog.showModal();
}

quickViewEditButton.addEventListener("click", () => {
  const card = quickViewCard;
  quickViewDialog.close();
  if (card) openEditDialog(card);
});

quickViewDialog.addEventListener("click", (event) => {
  if (event.target === quickViewDialog) quickViewDialog.close();
});

quickViewDialog.addEventListener("close", () => {
  quickViewCard = null;
  delete quickViewDeleteButton.dataset.deleteUrl;
});

quickViewDeleteButton.addEventListener("htmx:configRequest", (event) => {
  if (quickViewDeleteButton.dataset.deleteUrl) event.detail.path = quickViewDeleteButton.dataset.deleteUrl;
});

document.body.addEventListener("click", (event) => {
  if (event.target.closest("button, a, input, select, textarea, .event-quick-action")) return;
  const card = event.target.closest(".event-card");
  if (card) openQuickView(card);
});

document.body.addEventListener("keydown", (event) => {
  if (event.key !== "Enter" && event.key !== " ") return;
  const trigger = event.target.closest(".event-card-open");
  if (!trigger || event.target !== trigger) return;
  event.preventDefault();
  openQuickView(trigger.closest(".event-card"));
});

editCloseButton.addEventListener("click", () => editDialog.close());

editDialog.addEventListener("click", (event) => {
  if (event.target === editDialog) editDialog.close();
});

editDialog.addEventListener("close", () => {
  editForm.reset();
  delete editForm.dataset.patchUrl;
  delete editDeleteButton.dataset.deleteUrl;
  editFormBaseline = "";
  editSaveButton.disabled = true;
  editSections.forEach((section) => setSectionEnabled(section, false));
  hideDialogError(editDialog);
});

editForm.addEventListener("htmx:configRequest", (event) => {
  if (editForm.dataset.patchUrl) event.detail.path = editForm.dataset.patchUrl;
});

editDeleteButton.addEventListener("htmx:configRequest", (event) => {
  if (editDeleteButton.dataset.deleteUrl) event.detail.path = editDeleteButton.dataset.deleteUrl;
});

editForm.addEventListener("input", queueEditSaveStateUpdate);
editForm.addEventListener("change", queueEditSaveStateUpdate);

function onEventEdited() {
  editDialog.close();
}

window.onEventEdited = onEventEdited;

function onEventDeleted() {
  editDialog.close();
  quickViewDialog.close();
}

window.onEventDeleted = onEventDeleted;

document.body.addEventListener("input", (event) => {
  const form = event.target.closest("form");
  if (!form) return;

  if (event.target.matches("[data-sleep-duration-minutes]")) {
    updateSleepEndFromDuration(form);
    return;
  }

  if (event.target.matches("[data-feed-duration-minutes]")) {
    updateFeedEndFromDuration(form);
    return;
  }

  if (event.target.matches("[data-pump-duration-minutes]")) {
    updatePumpEndFromDuration(form);
    return;
  }

  if (event.target.closest("[data-sleep-time-fields]") || event.target.matches('input[name="date"], input[name="time"]')) {
    updateSleepDuration(form);
  }

  if (event.target.closest("[data-feed-time-fields]") || event.target.matches('input[name="date"], input[name="time"]')) {
    updateFeedDuration(form);
  }

  if (event.target.closest("[data-pump-time-fields]") || event.target.matches('input[name="date"], input[name="time"]')) {
    updatePumpDuration(form);
  }
});

document.body.addEventListener("change", (event) => {
  if (event.target.matches('input[type="radio"][name="kind"]')) {
    updatePooSizeFields(event.target.closest("form"));
  }
  if (event.target.matches('input[type="radio"][name="type"]')) {
    updateFeedAmountFields(event.target.closest("form"));
  }
});

// The day-range links swap #timeline-workspace via htmx instead of a full
// page reload, so the day pills themselves are never re-rendered by the
// server on click — move the active/current state here instead, the same
// way the type filter chips already update themselves client-side. Browser
// back/forward is a real page reload (see htmx.config above), so the
// server-rendered active state already covers that path and needs no
// client-side sync here.
document.body.addEventListener("click", (event) => {
  const pill = event.target.closest(".range-pill");
  if (!pill) return;
  const rangeNav = pill.closest(".range-nav");
  if (!rangeNav) return;

  rangeNav.querySelectorAll(".range-pill").forEach((candidate) => {
    const isSelected = candidate === pill;
    candidate.classList.toggle("active", isSelected);
    if (isSelected) {
      candidate.setAttribute("aria-current", "page");
    } else {
      candidate.removeAttribute("aria-current");
    }
  });
});

// The add-event and edit-event dialogs sit outside #timeline-workspace (they
// need to survive being open across a day switch), so each of their hidden
// selected_date fields was only ever set once, from whatever day was active
// on the original full page load. Switching days now just swaps
// #timeline-workspace via htmx rather than reloading the page, so without
// this those fields go stale: create/edit/delete an event after switching
// days and it silently re-targets the day you *started* the session on,
// while the day pill correctly shows the day you actually switched to. The
// URL's own date param is kept current by hx-push-url on every day-pill
// click, so it's the reliable source of truth here — re-synced on every
// #timeline-workspace swap, which also covers plain event mutations (where
// the URL doesn't change, so this is a harmless no-op reaffirming the same
// value).
document.body.addEventListener("htmx:afterSwap", (event) => {
  if (event.target.id !== "timeline-workspace") return;
  const selectedDate = new URLSearchParams(window.location.search).get("date");
  if (!selectedDate) return;

  document.querySelectorAll('input[name="selected_date"]').forEach((input) => {
    if (!input.closest("#timeline-workspace")) input.value = selectedDate;
  });
});

// Timeline navigation and display filters live inside a collapsible section
// so they don't take up screen space when the timeline itself is what's
// wanted. Collapsed is the default; the expand/collapse state is remembered
// per device, same as the type filter below.

const filtersToggle = document.getElementById("timeline-filters-toggle");
const filtersBody = document.getElementById("timeline-filters-body");
const FILTERS_EXPANDED_STORAGE_KEY = "yauli-filters-expanded";

function setFiltersExpanded(expanded) {
  filtersBody.hidden = !expanded;
  filtersToggle.setAttribute("aria-expanded", String(expanded));
  filtersToggle.setAttribute("aria-label", expanded ? "Hide timeline filters" : "Show timeline filters");
  try {
    localStorage.setItem(FILTERS_EXPANDED_STORAGE_KEY, expanded ? "1" : "0");
  } catch {
    // Storage can be unavailable (e.g. private browsing) - the toggle still
    // works for the current page view, it just won't be remembered.
  }
}

function loadFiltersExpanded() {
  try {
    return localStorage.getItem(FILTERS_EXPANDED_STORAGE_KEY) === "1";
  } catch {
    return false;
  }
}

if (filtersToggle && filtersBody) {
  setFiltersExpanded(loadFiltersExpanded());

  filtersToggle.addEventListener("click", () => {
    setFiltersExpanded(filtersBody.hidden);
  });
}

// Event type filter: purely client-side, hiding/showing already-rendered
// cards, so switching types is instant and needs no round trip to the
// backend. The selection is remembered per device so it doesn't reset every
// time the page loads or the selected date changes.

const typeFilter = document.getElementById("type-filter");
const TYPE_FILTER_STORAGE_KEY = "yauli-event-filter";

function loadStoredEventFilter() {
  try {
    const types = JSON.parse(localStorage.getItem(TYPE_FILTER_STORAGE_KEY) || "[]");
    return Array.isArray(types) ? types : [];
  } catch {
    return [];
  }
}

function storeEventFilter(types) {
  try {
    localStorage.setItem(TYPE_FILTER_STORAGE_KEY, JSON.stringify(types));
  } catch {
    // Storage can be unavailable (e.g. private browsing) - the filter still
    // works for the current page view, it just won't be remembered.
  }
}

function activeEventFilterTypes() {
  return Array.from(typeFilter.querySelectorAll('.type-filter-chip.active[data-filter-type]:not([data-filter-type="all"])'))
    .map((chip) => chip.dataset.filterType);
}

function setActiveEventFilterChips(types) {
  const hasSelection = types.length > 0;
  typeFilter.querySelectorAll(".type-filter-chip").forEach((chip) => {
    const isAll = chip.dataset.filterType === "all";
    chip.classList.toggle("active", isAll ? !hasSelection : types.includes(chip.dataset.filterType));
  });
}

function applyEventFilter() {
  const activeTypes = activeEventFilterTypes();
  const cards = Array.from(document.querySelectorAll("#timeline .event-card"));
  let anyVisible = false;
  cards.forEach((card) => {
    const visible = activeTypes.length === 0 || activeTypes.includes(card.dataset.eventType);
    card.hidden = !visible;
    if (visible) anyVisible = true;
  });

  const filterEmptyMessage = document.getElementById("timeline-filter-empty");
  if (filterEmptyMessage) filterEmptyMessage.hidden = cards.length === 0 || anyVisible;
}

if (typeFilter) {
  setActiveEventFilterChips(loadStoredEventFilter());
  applyEventFilter();

  typeFilter.addEventListener("click", (event) => {
    const chip = event.target.closest(".type-filter-chip");
    if (!chip) return;

    let types;
    if (chip.dataset.filterType === "all") {
      types = [];
    } else {
      const selected = new Set(activeEventFilterTypes());
      if (selected.has(chip.dataset.filterType)) {
        selected.delete(chip.dataset.filterType);
      } else {
        selected.add(chip.dataset.filterType);
      }
      types = Array.from(selected);
    }

    setActiveEventFilterChips(types);
    storeEventFilter(types);
    applyEventFilter();
  });

  // Re-apply the filter every time htmx swaps in fresh timeline markup
  // (after creating, editing, deleting, or finishing an event), since the
  // new cards start with every type visible.
  document.body.addEventListener("htmx:afterSwap", (event) => {
    if (event.target.id === "timeline" || event.target.id === "timeline-workspace") applyEventFilter();
  });
}
