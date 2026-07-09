// Native form.reset() restores date/time inputs to the value attribute
// rendered when the page loaded, not the current time — so logging a second
// event later in the same session without reloading would silently misdate
// it. Reset the rest of the form natively, then overwrite date/time with the
// browser's current local date/time.
//
// This assumes the browser's local timezone matches the baby's configured
// timezone (Australia/Adelaide) rather than asking the server, which is fine
// for a single-family app used from devices in that timezone.
function resetFormKeepingNow(form) {
  form.reset();

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
