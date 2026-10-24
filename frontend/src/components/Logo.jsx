export default function Logo({ width = 40, height = 32 }) {
  return (
    <svg viewBox="0 0 40 32" width={width} height={height} aria-hidden="true">
      <path fill="currentColor" d="M0 14 8 0h24l8 14-6 4-5-9H11l-5 9zM6 24l6-4h16l6 4-4 8H10z" />
    </svg>
  );
}
