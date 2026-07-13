type LoginHologramShieldIconProps = {
  className?: string;
};

const dotRows = [
  { y: 47, start: 82, end: 138, step: 8, opacity: 0.16 },
  { y: 55, start: 70, end: 150, step: 8, opacity: 0.22 },
  { y: 63, start: 62, end: 158, step: 8, opacity: 0.26 },
  { y: 71, start: 58, end: 162, step: 8, opacity: 0.25 },
  { y: 79, start: 54, end: 166, step: 8, opacity: 0.24 },
  { y: 87, start: 52, end: 168, step: 8, opacity: 0.23 },
  { y: 95, start: 52, end: 168, step: 8, opacity: 0.22 },
  { y: 103, start: 54, end: 166, step: 8, opacity: 0.2 },
  { y: 111, start: 56, end: 164, step: 8, opacity: 0.18 },
  { y: 119, start: 60, end: 160, step: 8, opacity: 0.16 },
  { y: 127, start: 64, end: 156, step: 8, opacity: 0.14 },
  { y: 135, start: 70, end: 150, step: 8, opacity: 0.12 },
];

function buildDots() {
  return dotRows.flatMap((row) => {
    const dots = [];
    for (let x = row.start; x <= row.end; x += row.step) {
      dots.push(<circle key={`${row.y}-${x}`} cx={x} cy={row.y} r="1.25" opacity={row.opacity} />);
    }
    return dots;
  });
}

export function LoginHologramShieldIcon({ className }: LoginHologramShieldIconProps) {
  return (
    <svg
      className={className}
      viewBox="0 0 220 260"
      role="img"
      aria-label="园区安全认证盾牌"
      focusable="false"
    >
      <g>
        <path
          d="M110 22 L178 52 L170 154 C157 178 139 198 110 219 C81 198 63 178 50 154 L42 52 Z"
          fill="#0c66c7"
          fillOpacity="0.075"
          stroke="#0a72d8"
          strokeOpacity="0.24"
          strokeWidth="7"
          strokeLinejoin="round"
        />
        <path
          d="M110 22 L178 52 L170 154 C157 178 139 198 110 219 C81 198 63 178 50 154 L42 52 Z"
          fill="#0a4d99"
          fillOpacity="0.13"
          stroke="#42b7ff"
          strokeOpacity="0.54"
          strokeWidth="2.2"
          strokeLinejoin="round"
        />
        <g>
          <path d="M110 23 V220" stroke="#2ba7ff" strokeOpacity="0.22" strokeWidth="1" />
          <path d="M42 52 L110 80 L178 52" fill="none" stroke="#5ec4ff" strokeOpacity="0.28" strokeWidth="1.2" />
          <path d="M50 154 C76 165 144 165 170 154" fill="none" stroke="#088cff" strokeOpacity="0.32" strokeWidth="1" />
          <g fill="#1aa5ff">{buildDots()}</g>
          <path d="M44 91 C78 80 142 80 176 91" fill="none" stroke="#1b9fff" strokeOpacity="0.16" />
          <path d="M51 131 C81 142 139 142 169 131" fill="none" stroke="#1b9fff" strokeOpacity="0.16" />
        </g>
        <g transform="translate(110 132) scale(0.82) translate(-110 -132)">
          <path
            d="M110 77 L147 92 L143 149 C134 163 123 176 110 186 C97 176 86 163 77 149 L73 92 Z"
            fill="rgba(3, 26, 58, 0.18)"
            stroke="#d9ecff"
            strokeOpacity="0.5"
            strokeWidth="4.6"
            strokeLinejoin="round"
          />
          <g
            fill="none"
            stroke="#e8f5ff"
            strokeOpacity="0.66"
            strokeWidth="6.2"
            strokeLinejoin="miter"
            strokeLinecap="square"
          >
            <path d="M110 91 V159" />
            <path d="M86 106 H134 V134 H86 Z" />
            <path d="M86 120 H134" />
          </g>
          <path d="M90 79 L110 70 L130 79" fill="none" stroke="#d9ecff" strokeOpacity="0.38" strokeWidth="4.4" />
          <path d="M100 198 L110 210 L120 198" fill="#d9ecff" fillOpacity="0.52" />
        </g>
      </g>
    </svg>
  );
}
