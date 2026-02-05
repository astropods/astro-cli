import type { Meta, StoryObj } from '@storybook/react-vite'

const meta = {
  title: 'Example/Button',
  component: () => <button className="bg-primary text-primary-foreground px-4 py-2 rounded-md">Button</button>,
} satisfies Meta

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {}
