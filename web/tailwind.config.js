/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{js,ts,jsx,tsx}"],
  theme: {
    extend: {
      colors: {
        bg: {
          DEFAULT: '#0a0e14',
          elevated: '#131b24',
          overlay: '#1a2332',
          input: '#0d1117',
        },
        border: {
          DEFAULT: '#1e2d3d',
          active: '#2dd4bf',
          focus: '#14b8a6',
        },
        text: {
          DEFAULT: '#e6edf3',
          muted: '#8b949e',
          dimmed: '#484f58',
        },
        accent: {
          DEFAULT: '#10b981',
          hover: '#34d399',
          dimmed: '#064e3b',
        },
        node: {
          trigger: '#f59e0b',
          action: '#10b981',
          logic: '#8b5cf6',
          llm: '#ec4899',
          data: '#06b6d4',
        }
      },
      fontFamily: {
        mono: ['JetBrains Mono', 'Fira Code', 'monospace'],
      },
      keyframes: {
        'slide-in': {
          '0%': { transform: 'translateX(100%)', opacity: '0' },
          '100%': { transform: 'translateX(0)', opacity: '1' },
        },
        'fade-in': {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' },
        },
        'dock-in': {
          '0%': { opacity: '0', transform: 'translateY(18px) scale(0.97)' },
          '100%': { opacity: '1', transform: 'translateY(0) scale(1)' },
        },
        'picker-in': {
          '0%': { opacity: '0', transform: 'translateY(10px) scale(0.98)' },
          '100%': { opacity: '1', transform: 'translateY(0) scale(1)' },
        },
      },
      animation: {
        'slide-in': 'slide-in 0.2s ease-out',
        'fade-in': 'fade-in 0.15s ease-out',
        'dock-in': 'dock-in 0.2s cubic-bezier(0.22, 1, 0.36, 1)',
        'picker-in': 'picker-in 0.16s cubic-bezier(0.22, 1, 0.36, 1)',
      },
    },
  },
  plugins: [],
}
