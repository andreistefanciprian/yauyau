// Drives the single "Add Event" dialog: step 1 picks an event type, step 2
// shows only that type's form. Each form posts straight to its existing
// endpoint (/nappies, /feeds, /pumps, /baths, /sleeps, /observations, /temperatures) via htmx and swaps in
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
  updateSleepDuration(form);
  updatePooSizeFields(form);
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
const editTypeInput = document.getElementById("edit-event-type");
const editSections = Array.from(document.querySelectorAll(".edit-event-fields"));
const editTitle = document.getElementById("edit-event-title");
const editTimeLabel = editForm.querySelector("[data-edit-time-label]");
const editDateLabel = editForm.querySelector("[data-edit-date-label]");
const editOccurredAtFields = editForm.querySelector("[data-edit-occurred-at-fields]");
const editOccurredAtLabel = editForm.querySelector("[data-edit-occurred-at-label]");

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

function updateSleepDuration(scope) {
  const fields = scope.querySelectorAll("[data-sleep-time-fields]");
  fields.forEach((container) => {
    const form = container.closest("form");
    if (!form) return;

    const startDate = form.querySelector('input[name="date"]');
    const startTime = form.querySelector('input[name="time"]');
    const endDate = container.querySelector("[data-sleep-end-date]");
    const endTime = container.querySelector("[data-sleep-end-time]");
    const durationInput = container.parentElement.querySelector("[data-sleep-duration-minutes]");
    const preview = container.querySelector("[data-sleep-duration-preview]");
    const start = parseLocalDateTime(startDate?.value, startTime?.value);
    const end = parseLocalDateTime(endDate?.value, endTime?.value);

    endDate.setCustomValidity("");
    endTime.setCustomValidity("");

    if (!start || !end) {
      if (durationInput) durationInput.value = "";
      if (preview) preview.textContent = "Add wake-up time to calculate duration.";
      return;
    }

    if (end <= start) {
      const message = "Wake-up time must be after sleep start.";
      endTime.setCustomValidity(message);
      if (durationInput) durationInput.value = "";
      if (preview) preview.textContent = message;
      return;
    }

    const minutes = Math.round((end - start) / 60000);
    if (durationInput) durationInput.value = String(minutes);
    if (preview) preview.textContent = `Duration: ${formatDuration(minutes)}`;
  });
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
  const isSleep = type === "sleep";
  if (editOccurredAtFields) editOccurredAtFields.classList.toggle("sleep-edit-time-fields", isSleep);
  if (editOccurredAtLabel) editOccurredAtLabel.hidden = !isSleep;
  if (editTimeLabel) editTimeLabel.textContent = "Time";
  if (editDateLabel) editDateLabel.textContent = "Date";

  editSections.forEach((section) => {
    setSectionEnabled(section, section.dataset.editType === type);
  });
  const activeSection = editSectionForType(type);
  if (!activeSection) return;

  const editDateInput = editForm.querySelector('input[type="date"]');
  if (editDateInput) editDateInput.max = localDateValue(new Date());

  setFieldValue(editForm, "date", button.dataset.date);
  setFieldValue(editForm, "time", button.dataset.time);

  switch (type) {
    case "nappy":
      setRadioValue(activeSection, "kind", button.dataset.kind, "wet");
      setRadioValue(activeSection, "poo_size", button.dataset.pooSize, "medium");
      setCheckboxValues(activeSection, "labels", button.dataset.labels);
      updatePooSizeFields(editForm);
      setFieldValue(activeSection, "notes", button.dataset.notes);
      break;
    case "feed":
      setRadioValue(activeSection, "type", button.dataset.type, "breast");
      setRadioValue(activeSection, "feed_time_basis", "start", "start");
      setFieldValue(activeSection, "amount_ml", button.dataset.amountMl);
      setFieldValue(activeSection, "duration_minutes", button.dataset.durationMinutes);
      setFieldValue(activeSection, "notes", button.dataset.notes);
      break;
    case "pump":
      setFieldValue(activeSection, "amount_ml", button.dataset.amountMl);
      setFieldValue(activeSection, "notes", button.dataset.notes);
      break;
    case "bath":
      setRadioValue(activeSection, "type", button.dataset.type, "whole_body");
      setRadioValue(activeSection, "bath_time_basis", "start", "start");
      setFieldValue(activeSection, "notes", button.dataset.notes);
      setFieldValue(activeSection, "duration_minutes", button.dataset.durationMinutes);
      break;
    case "sleep":
      setRadioValue(activeSection, "type", button.dataset.type, "nap");
      setFieldValue(activeSection, "notes", button.dataset.notes);
      setFieldValue(activeSection, "duration_minutes", button.dataset.durationMinutes);
      setSleepEndFromStart(editForm, button.dataset.durationMinutes);
      updateSleepDuration(editForm);
      break;
    case "observation":
      setFieldValue(activeSection, "text", button.dataset.text);
      setFieldValue(activeSection, "category", button.dataset.category);
      break;
    case "temperature":
      setFieldValue(activeSection, "temperature_c", button.dataset.temperatureC);
      setFieldValue(activeSection, "method", button.dataset.method);
      setFieldValue(activeSection, "notes", button.dataset.notes);
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
  hideDialogError(editDialog);
});

editForm.addEventListener("htmx:configRequest", (event) => {
  if (editForm.dataset.patchUrl) event.detail.path = editForm.dataset.patchUrl;
});

function onEventEdited() {
  editDialog.close();
}

window.onEventEdited = onEventEdited;

document.body.addEventListener("input", (event) => {
  const form = event.target.closest("form");
  if (!form) return;

  if (event.target.closest("[data-sleep-time-fields]") || event.target.matches('input[name="date"], input[name="time"]')) {
    updateSleepDuration(form);
  }
});

document.body.addEventListener("change", (event) => {
  if (event.target.matches('input[type="radio"][name="kind"]')) {
    updatePooSizeFields(event.target.closest("form"));
  }
});

// The day-range nav and event-type filter live inside a collapsible section
// so they don't take up screen space when the timeline itself is what's
// wanted. Collapsed is the default; the expand/collapse state is remembered
// per device, same as the type filter below.

const filtersToggle = document.getElementById("timeline-filters-toggle");
const filtersBody = document.getElementById("timeline-filters-body");
const filtersSummary = document.getElementById("timeline-filters-summary");
const FILTERS_EXPANDED_STORAGE_KEY = "yauli-filters-expanded";

const typeFilterChipLabels = {
  nappy: "Nappy",
  feed: "Feed",
  pump: "Pump",
  bath: "Bath",
  sleep: "Sleep",
  observation: "Notes",
  temperature: "Temperature",
};

function setFiltersExpanded(expanded) {
  filtersBody.hidden = !expanded;
  filtersToggle.setAttribute("aria-expanded", String(expanded));
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

function updateFiltersSummary() {
  const activeRange = document.querySelector(".range-pill.active");
  const rangeLabel = activeRange ? activeRange.textContent.trim() : "Today";

  const activeTypes = activeEventFilterTypes();
  const typeLabel = activeTypes.length === 0
    ? "All events"
    : activeTypes.map((type) => typeFilterChipLabels[type] || type).join(", ");

  filtersSummary.textContent = `${rangeLabel} · ${typeLabel}`;
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
// time the page loads or the date range changes.

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
  if (filtersSummary) updateFiltersSummary();

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
    if (filtersSummary) updateFiltersSummary();
  });

  // Re-apply the filter every time htmx swaps in a fresh #timeline (after
  // creating, editing, or deleting an event), since the new markup starts
  // with every card visible.
  document.body.addEventListener("htmx:afterSwap", (event) => {
    if (event.target.id === "timeline") applyEventFilter();
  });
}

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
