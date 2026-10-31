import * as React from 'react'
import * as DialogPrimitive from '@radix-ui/react-dialog'
import * as DropdownPrimitive from '@radix-ui/react-dropdown-menu'
import * as SwitchPrimitive from '@radix-ui/react-switch'
import * as TooltipPrimitive from '@radix-ui/react-tooltip'
import { Slot } from '@radix-ui/react-slot'
import { cva, type VariantProps } from 'class-variance-authority'
import { Check, ChevronDown, X } from 'lucide-react'
import { cn } from '../../lib/utils'

// shadcn/ui conventions: local components, Radix accessibility, CVA variants.
const buttonVariants = cva('ui-button', {
  variants: {
    variant: {
      default: 'ui-button-default',
      secondary: 'ui-button-secondary',
      ghost: 'ui-button-ghost',
      outline: 'ui-button-outline',
      danger: 'ui-button-danger',
    },
    size: { default: '', sm: 'ui-button-sm', icon: 'ui-button-icon' },
  },
  defaultVariants: { variant: 'default', size: 'default' },
})

export function Button({
  className,
  variant,
  size,
  asChild = false,
  ...props
}: React.ComponentProps<'button'> & VariantProps<typeof buttonVariants> & { asChild?: boolean }) {
  const Component = asChild ? Slot : 'button'
  return (
    <Component
      type="button"
      className={cn(buttonVariants({ variant, size }), className)}
      {...props}
    />
  )
}

export function Input({ className, ...props }: React.ComponentProps<'input'>) {
  return <input className={cn('ui-input', className)} {...props} />
}

export function Badge({
  className,
  tone = 'neutral',
  ...props
}: React.ComponentProps<'span'> & { tone?: 'neutral' | 'green' | 'blue' | 'amber' }) {
  return <span className={cn('ui-badge', `ui-badge-${tone}`, className)} {...props} />
}

export function Switch(props: React.ComponentProps<typeof SwitchPrimitive.Root>) {
  return (
    <SwitchPrimitive.Root className="ui-switch" {...props}>
      <SwitchPrimitive.Thumb className="ui-switch-thumb" />
    </SwitchPrimitive.Root>
  )
}

export const TooltipProvider = TooltipPrimitive.Provider
export function Hint({ label, children }: { label: string; children: React.ReactElement }) {
  return (
    <TooltipPrimitive.Root>
      <TooltipPrimitive.Trigger asChild>{children}</TooltipPrimitive.Trigger>
      <TooltipPrimitive.Portal>
        <TooltipPrimitive.Content className="ui-tooltip" sideOffset={7}>
          {label}
          <TooltipPrimitive.Arrow className="ui-tooltip-arrow" />
        </TooltipPrimitive.Content>
      </TooltipPrimitive.Portal>
    </TooltipPrimitive.Root>
  )
}

export function Dialog({
  open,
  onOpenChange,
  title,
  description,
  children,
  className,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description: string
  children: React.ReactNode
  className?: string
}) {
  return (
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Overlay className="ui-dialog-overlay" />
        <DialogPrimitive.Content className={cn('ui-dialog', className)}>
          <div className="ui-dialog-heading">
            <DialogPrimitive.Title>{title}</DialogPrimitive.Title>
            <DialogPrimitive.Close asChild>
              <Button variant="ghost" size="icon" aria-label="关闭对话框">
                <X size={17} />
              </Button>
            </DialogPrimitive.Close>
          </div>
          <DialogPrimitive.Description className="ui-dialog-description">
            {description}
          </DialogPrimitive.Description>
          {children}
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  )
}

export function Menu({
  trigger,
  items,
  align = 'end',
}: {
  trigger: React.ReactNode
  items: {
    label: string
    onSelect?: () => void
    icon?: React.ReactNode
    checked?: boolean
    danger?: boolean
  }[]
  align?: 'start' | 'center' | 'end'
}) {
  return (
    <DropdownPrimitive.Root>
      <DropdownPrimitive.Trigger asChild>{trigger}</DropdownPrimitive.Trigger>
      <DropdownPrimitive.Portal>
        <DropdownPrimitive.Content className="ui-menu" align={align} sideOffset={7}>
          {items.map((item, index) => (
            <DropdownPrimitive.Item
              key={`${item.label}-${index}`}
              className={cn('ui-menu-item', item.danger && 'ui-menu-danger')}
              onSelect={item.onSelect}
            >
              {item.icon}
              <span>{item.label}</span>
              {item.checked && <Check size={14} className="menu-check" />}
            </DropdownPrimitive.Item>
          ))}
        </DropdownPrimitive.Content>
      </DropdownPrimitive.Portal>
    </DropdownPrimitive.Root>
  )
}

export function Select({
  label,
  value,
  onChange,
  options,
}: {
  label: string
  value: string
  onChange: (value: string) => void
  options: { value: string; label: string }[]
}) {
  return (
    <div className="ui-select-wrap">
      <select
        aria-label={label}
        className="ui-input ui-select"
        value={value}
        onChange={(event) => onChange(event.target.value)}
      >
        {options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
      <ChevronDown size={14} aria-hidden="true" />
    </div>
  )
}
