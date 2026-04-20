import { useColorScheme } from 'react-native';

const palette = {
  dark: {
    accent: '#42FF9C',
    accentDim: '#2BC87A',
    bg: '#050505',
    bgCard: 'rgba(0,0,0,0.55)',
    bgInput: 'rgba(0,0,0,0.45)',
    bgHover: 'rgba(255,255,255,0.06)',
    bgBadge: 'rgba(255,255,255,0.08)',
    bgCode: '#0d1117',
    border: 'rgba(255,255,255,0.08)',
    borderSubtle: 'rgba(255,255,255,0.05)',
    text: '#ffffff',
    textSecondary: '#a0a0ab',
    textTertiary: '#63636e',
    textQuaternary: '#3e3e47',
    danger: '#f87171',
  },
  light: {
    accent: '#16a34a',
    accentDim: '#15803d',
    bg: '#f8f8fa',
    bgCard: 'rgba(255,255,255,0.80)',
    bgInput: 'rgba(255,255,255,0.90)',
    bgHover: 'rgba(0,0,0,0.04)',
    bgBadge: 'rgba(0,0,0,0.05)',
    bgCode: '#fafafa',
    border: 'rgba(0,0,0,0.12)',
    borderSubtle: 'rgba(0,0,0,0.06)',
    text: '#111113',
    textSecondary: '#6b6b7a',
    textTertiary: '#9b9baa',
    textQuaternary: '#c5c5d0',
    danger: '#ef4444',
  },
};

export type Theme = typeof palette.dark;

export function useTheme(): Theme {
  const scheme = useColorScheme();
  return scheme === 'light' ? palette.light : palette.dark;
}

export const fonts = {
  sans: undefined, // system default
  mono: 'monospace',
};
