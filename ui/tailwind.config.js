/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,jsx}'],
  theme: {
    extend: {
      colors: {
        bg:       '#07090c',
        surface:  '#0f1318',
        surface2: '#141b22',
        border:   '#1c2730',
        borderhi: '#243444',
        dim:      '#2a3d4d',
        muted:    '#3d5568',
        text:     '#b8d0e0',
        bright:   '#e2f0fb',
        cyan:     '#00b8f5',
        cyandim:  '#00304d',
        green:    '#00d488',
        greendim: '#003d28',
        yellow:   '#f5c400',
        yellowdim:'#3d2f00',
        red:      '#ff3c3c',
        reddim:   '#3d0808',
        purple:   '#9d72ff',
      },
      fontFamily: {
        mono: ['"JetBrains Mono"', 'monospace'],
        cond: ['"Barlow Condensed"', 'sans-serif'],
      },
      animation: {
        pulse2: 'pulse2 2s ease-in-out infinite',
        blink:  'blink 0.9s ease-in-out infinite',
      },
      keyframes: {
        pulse2: { '0%,100%': { opacity: 1 }, '50%': { opacity: 0.45 } },
        blink:  { '0%,100%': { opacity: 1 }, '50%': { opacity: 0.2 } },
      },
    },
  },
  plugins: [],
}