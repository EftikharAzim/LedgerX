import { useLayoutEffect, useRef } from "react";
import { formatMoney } from "../lib/money";

// Preserve every digit. Scale only when the formatted amount exceeds its
// container; the surrounding card still controls the preferred font size.
export default function FitMoney({
  minor,
  currency,
}: {
  minor: number;
  currency: string;
}) {
  const container = useRef<HTMLSpanElement>(null);
  const value = useRef<HTMLSpanElement>(null);
  const text = formatMoney(minor, currency);
  useLayoutEffect(() => {
    const parent = container.current;
    const amount = value.current;
    if (!parent || !amount) return;
    const fit = () => {
      amount.style.fontSize = "";
      const available = parent.clientWidth;
      const width = amount.getBoundingClientRect().width;
      if (available > 0 && width > available) {
        const base = parseFloat(getComputedStyle(parent).fontSize);
        amount.style.fontSize = `${(base * available) / width}px`;
      }
    };
    fit();
    const observer = new ResizeObserver(fit);
    observer.observe(parent);
    return () => observer.disconnect();
  }, [text]);
  return (
    <span className="fit-money" ref={container}>
      <span ref={value}>{text}</span>
    </span>
  );
}
