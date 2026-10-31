import { useEffect, useRef } from "react";

export function useModalDialog() {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const opener =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    const dialog = ref.current;
    dialog?.showModal();
    dialog?.querySelector<HTMLElement>("[data-autofocus]")?.focus();

    // Native dialogs make the background inert. Explicitly wrap Tab as well:
    // browsers may otherwise move focus to their toolbar after the last item,
    // or to the body when a focused submit button becomes disabled.
    const containFocus = (event: KeyboardEvent) => {
      if (event.key !== "Tab" || !dialog?.open) return;
      const candidates = [
        ...dialog.querySelectorAll<HTMLElement>(
          "button, input, select, textarea, a[href], [tabindex]",
        ),
      ].filter(
        (el) =>
          el.tabIndex >= 0 &&
          !el.matches(":disabled") &&
          el.getClientRects().length > 0,
      );
      const focusable = candidates.filter(
        (el) =>
          !(
            el instanceof HTMLInputElement &&
            el.type === "radio" &&
            !el.checked &&
            candidates.some(
              (other) =>
                other instanceof HTMLInputElement &&
                other.type === "radio" &&
                other.name === el.name &&
                other.checked,
            )
          ),
      );
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      const outside = !dialog.contains(document.activeElement);
      if (!first) {
        event.preventDefault();
        dialog.focus();
      } else if (
        outside ||
        (!event.shiftKey && document.activeElement === last) ||
        (event.shiftKey && document.activeElement === first)
      ) {
        event.preventDefault();
        (event.shiftKey ? last : first).focus();
      }
    };
    document.addEventListener("keydown", containFocus, true);
    return () => {
      document.removeEventListener("keydown", containFocus, true);
      dialog?.close();
      if (opener?.isConnected) opener.focus({ preventScroll: true });
    };
  }, []);
  return ref;
}
