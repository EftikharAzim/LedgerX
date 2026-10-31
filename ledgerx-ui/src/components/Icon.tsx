import type { SVGProps } from "react";

const paths = {
  overview: "M3 3h7v7H3z M14 3h7v7h-7z M3 14h7v7H3z M14 14h7v7h-7z",
  wallet:
    "M20 8V6a2 2 0 0 0-2-2H6a3 3 0 0 0 0 6h14v10H6a3 3 0 0 1-3-3V7 M20 13h-5v4h5 M16.5 15h.01",
  bank: "m3 8 9-5 9 5H3 M5 11v7 M10 11v7 M14 11v7 M19 11v7 M3 21h18",
  card: "M3 5h18v14H3z M3 10h18 M7 15h3",
  coins:
    "M15 8c0 2-3 3-6 3S3 10 3 8s3-3 6-3 6 1 6 3Z M3 8v5c0 2 3 3 6 3 1 0 2 0 3-.5 M3 13v5c0 2 3 3 6 3 1 0 2 0 3-.5 M15 12c3 0 6 1 6 3s-3 3-6 3-6-1-6-3 M9 15v4c0 2 3 3 6 3s6-1 6-3v-4",
  export: "M14 3H5v18h14V8l-5-5Z M14 3v5h5 M12 11v6 m-3-3 3 3 3-3",
  plus: "M12 5v14 M5 12h14",
  arrow: "M5 12h14 m-5-5 5 5-5 5",
  up: "M7 17 17 7 M7 7h10v10",
  down: "M7 7 17 17 M7 17h10V7",
  check: "m5 12 4 4L19 6",
  close: "m6 6 12 12 M6 18 18 6",
  search: "M18 10a7 7 0 1 1-14 0 7 7 0 0 1 14 0Z m-2 6 5 5",
  grid: "M3 3h7v7H3z M14 3h7v7h-7z M3 14h7v7H3z M14 14h7v7h-7z",
  list: "M9 6h12 M9 12h12 M9 18h12 M3 6h.01 M3 12h.01 M3 18h.01",
  logout: "M9 4H4v16h5 M10 12h11 m-4-4 4 4-4 4",
  calendar: "M4 5h16v16H4z M8 3v4 M16 3v4 M4 10h16 M8 14h2 M14 14h2",
  chevron: "m9 5 7 7-7 7",
  shield: "m12 3 8 3v6c0 5-8 9-8 9s-8-4-8-9V6l8-3Z m-4 9 3 3 5-5",
  info: "M12 8h.01 M12 11v6 M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z",
  sparkle: "M12 2v20 M2 12h20 M5 5l14 14 M5 19 19 5",
  transfer: "M3 7h17 m-4-4 4 4-4 4 M21 17H4 m4-4-4 4 4 4",
  reverse: "M4 9h9a6 6 0 0 1 0 12 M4 9l5-5 M4 9l5 5",
  receipt: "M5 3h14v18l-3-2-4 2-4-2-3 2V3Z M9 8h6 M9 12h6",
  tag: "M3 3h8l10 10-8 8L3 11V3Z M7 7h.01",
  eye: "M2 12s4-7 10-7 10 7 10 7-4 7-10 7S2 12 2 12Z M15 12a3 3 0 1 1-6 0 3 3 0 0 1 6 0Z",
  eyeOff:
    "m3 3 18 18 M10 5c7-2 12 7 12 7s-1 2-3 3 M6 6c-3 2-4 6-4 6s4 7 10 7c2 0 4-1 5-2 M10 10a3 3 0 0 0 4 4",
  download: "M12 3v12 m-5-5 5 5 5-5 M4 16v5h16v-5",
  clock: "M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z M12 7v5l3 2",
} as const;

export type IconName = keyof typeof paths;

export default function Icon({
  name,
  ...props
}: SVGProps<SVGSVGElement> & { name: IconName }) {
  return (
    <svg
      width="20"
      height="20"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      {...props}
    >
      <path d={paths[name]} />
    </svg>
  );
}

export function BrandMark() {
  return (
    <span className="brand-mark" aria-hidden="true">
      <span />
      <span />
    </span>
  );
}
