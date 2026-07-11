// Shared numeric input controls (a +/- stepper, and a drag slider paired
// with a precise number field), used on the Add/Edit event forms and on the
// Settings page's birth weight and length fields. Kept separate from app.js
// since app.js assumes the Add/Edit event dialogs are present on the page,
// which isn't true here.

function decimalPlaces(value) {
  const str = String(value);
  const dot = str.indexOf(".");
  return dot === -1 ? 0 : str.length - dot - 1;
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
  // Fractional steps (e.g. 0.1) hit binary floating-point drift (3.12 + 0.1
  // = 3.2199999999999998), so round to the input's own decimal precision
  // rather than printing the raw float.
  const precision = Math.max(decimalPlaces(button.dataset.stepAmount), decimalPlaces(current));
  input.value = next.toFixed(precision);
  input.dispatchEvent(new Event("input", { bubbles: true }));
}

document.body.addEventListener("click", (event) => {
  const stepperButton = event.target.closest(".number-stepper-button");
  if (stepperButton) adjustNumberStepper(stepperButton);
});

// Drag slider paired with a precise number field: dragging the slider
// writes the rounded value into the number field, and typing into the
// number field moves the slider's thumb (clamped to its own, narrower
// drag range - the number field can still hold a value outside that range,
// the slider just parks at whichever end is closest).
function bindNumberSlider(container) {
  const range = container.querySelector(".number-slider-range");
  const input = container.querySelector(".number-slider-input");
  if (!range || !input) return;

  range.addEventListener("input", () => {
    input.value = range.value;
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });

  input.addEventListener("input", () => {
    const value = Number(input.value);
    if (input.value === "" || Number.isNaN(value)) return;
    const min = Number(range.min);
    const max = Number(range.max);
    range.value = String(Math.min(max, Math.max(min, value)));
  });
}

document.querySelectorAll(".number-slider").forEach(bindNumberSlider);
