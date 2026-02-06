import type { Meta, StoryObj } from '@storybook/react-vite'

const hues = [
  'indigo',
  'gray',
  'red',
  'amber',
  'green',
  'teal',
  'blue',
  'purple',
  'pink',
] as const

const shades = [50, 100, 200, 300, 400, 500, 600, 700, 800, 900, 950] as const

// Pre-computed class map so Tailwind can detect every utility at build time.
const bgClass: Record<string, Record<number, string>> = {
  indigo: {
    50: 'bg-indigo-50',
    100: 'bg-indigo-100',
    200: 'bg-indigo-200',
    300: 'bg-indigo-300',
    400: 'bg-indigo-400',
    500: 'bg-indigo-500',
    600: 'bg-indigo-600',
    700: 'bg-indigo-700',
    800: 'bg-indigo-800',
    900: 'bg-indigo-900',
    950: 'bg-indigo-950',
  },
  gray: {
    50: 'bg-gray-50',
    100: 'bg-gray-100',
    200: 'bg-gray-200',
    300: 'bg-gray-300',
    400: 'bg-gray-400',
    500: 'bg-gray-500',
    600: 'bg-gray-600',
    700: 'bg-gray-700',
    800: 'bg-gray-800',
    900: 'bg-gray-900',
    950: 'bg-gray-950',
  },
  red: {
    50: 'bg-red-50',
    100: 'bg-red-100',
    200: 'bg-red-200',
    300: 'bg-red-300',
    400: 'bg-red-400',
    500: 'bg-red-500',
    600: 'bg-red-600',
    700: 'bg-red-700',
    800: 'bg-red-800',
    900: 'bg-red-900',
    950: 'bg-red-950',
  },
  amber: {
    50: 'bg-amber-50',
    100: 'bg-amber-100',
    200: 'bg-amber-200',
    300: 'bg-amber-300',
    400: 'bg-amber-400',
    500: 'bg-amber-500',
    600: 'bg-amber-600',
    700: 'bg-amber-700',
    800: 'bg-amber-800',
    900: 'bg-amber-900',
    950: 'bg-amber-950',
  },
  green: {
    50: 'bg-green-50',
    100: 'bg-green-100',
    200: 'bg-green-200',
    300: 'bg-green-300',
    400: 'bg-green-400',
    500: 'bg-green-500',
    600: 'bg-green-600',
    700: 'bg-green-700',
    800: 'bg-green-800',
    900: 'bg-green-900',
    950: 'bg-green-950',
  },
  teal: {
    50: 'bg-teal-50',
    100: 'bg-teal-100',
    200: 'bg-teal-200',
    300: 'bg-teal-300',
    400: 'bg-teal-400',
    500: 'bg-teal-500',
    600: 'bg-teal-600',
    700: 'bg-teal-700',
    800: 'bg-teal-800',
    900: 'bg-teal-900',
    950: 'bg-teal-950',
  },
  blue: {
    50: 'bg-blue-50',
    100: 'bg-blue-100',
    200: 'bg-blue-200',
    300: 'bg-blue-300',
    400: 'bg-blue-400',
    500: 'bg-blue-500',
    600: 'bg-blue-600',
    700: 'bg-blue-700',
    800: 'bg-blue-800',
    900: 'bg-blue-900',
    950: 'bg-blue-950',
  },
  purple: {
    50: 'bg-purple-50',
    100: 'bg-purple-100',
    200: 'bg-purple-200',
    300: 'bg-purple-300',
    400: 'bg-purple-400',
    500: 'bg-purple-500',
    600: 'bg-purple-600',
    700: 'bg-purple-700',
    800: 'bg-purple-800',
    900: 'bg-purple-900',
    950: 'bg-purple-950',
  },
  pink: {
    50: 'bg-pink-50',
    100: 'bg-pink-100',
    200: 'bg-pink-200',
    300: 'bg-pink-300',
    400: 'bg-pink-400',
    500: 'bg-pink-500',
    600: 'bg-pink-600',
    700: 'bg-pink-700',
    800: 'bg-pink-800',
    900: 'bg-pink-900',
    950: 'bg-pink-950',
  },
}

function ColorGrid() {
  return (
    <div className="p-6 font-sans">
      <h2 className="mb-6 text-2xl font-semibold text-foreground">
        Color Palette
      </h2>
      <div
        className="inline-grid gap-1"
        style={{
          gridTemplateColumns: `100px repeat(${shades.length}, auto)`,
        }}
      >
        {/* Header row: empty cell + shade numbers */}
        <div />
        {shades.map((shade) => (
          <div
            key={shade}
            className="pb-2 text-center text-xs font-semibold text-foreground"
          >
            {shade}
          </div>
        ))}

        {/* Color rows */}
        {hues.map((hue) => (
          <>
            {/* Row label */}
            <div
              key={`${hue}-label`}
              className="flex items-center text-sm font-medium capitalize text-foreground"
            >
              {hue}
            </div>
            {/* Swatches */}
            {shades.map((shade) => (
              <div
                key={`${hue}-${shade}`}
                className={`size-20 rounded-md border border-black/8 ${bgClass[hue][shade]}`}
                title={`${hue}-${shade}`}
              />
            ))}
          </>
        ))}
      </div>
    </div>
  )
}

const meta = {
  title: 'Theme/Colors',
  component: ColorGrid,
  parameters: {
    layout: 'fullscreen',
  },
} satisfies Meta<typeof ColorGrid>

export default meta
type Story = StoryObj<typeof meta>

export const Palette: Story = {}
