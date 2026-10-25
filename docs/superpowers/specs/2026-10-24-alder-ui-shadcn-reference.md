# shadcn/ui reference for the alder-ui restyle (companion to the design spec)

Fetched 2026-10-24 from `shadcn-ui/ui` `main`: `apps/v4/content/docs/(root)/theming.mdx`
(the canonical neutral theme), `apps/v4/app/globals.css` (the docs site) and
`apps/v4/registry/new-york-v4/ui/*.tsx`. Class strings are copied verbatim from the `cva`
and `cn` calls; a builder translates them with the rules in the design spec, section 5.
Where the builder needs more than is here (a component not listed, a sub-part), fetch the
registry file at `https://raw.githubusercontent.com/shadcn-ui/ui/main/apps/v4/registry/new-york-v4/ui/<name>.tsx`.

## 1. Token sheet (`theming.mdx`, verbatim)

```css
:root {
  --radius: 0.625rem;
  --background: oklch(1 0 0);
  --foreground: oklch(0.145 0 0);
  --card: oklch(1 0 0);
  --card-foreground: oklch(0.145 0 0);
  --popover: oklch(1 0 0);
  --popover-foreground: oklch(0.145 0 0);
  --primary: oklch(0.205 0 0);
  --primary-foreground: oklch(0.985 0 0);
  --secondary: oklch(0.97 0 0);
  --secondary-foreground: oklch(0.205 0 0);
  --muted: oklch(0.97 0 0);
  --muted-foreground: oklch(0.556 0 0);
  --accent: oklch(0.97 0 0);
  --accent-foreground: oklch(0.205 0 0);
  --destructive: oklch(0.577 0.245 27.325);
  --border: oklch(0.922 0 0);
  --input: oklch(0.922 0 0);
  --ring: oklch(0.708 0 0);
  --chart-1: oklch(0.646 0.222 41.116);
  --chart-2: oklch(0.6 0.118 184.704);
  --chart-3: oklch(0.398 0.07 227.392);
  --chart-4: oklch(0.828 0.189 84.429);
  --chart-5: oklch(0.769 0.188 70.08);
  --sidebar: oklch(0.985 0 0);
  --sidebar-foreground: oklch(0.145 0 0);
  --sidebar-primary: oklch(0.205 0 0);
  --sidebar-primary-foreground: oklch(0.985 0 0);
  --sidebar-accent: oklch(0.97 0 0);
  --sidebar-accent-foreground: oklch(0.205 0 0);
  --sidebar-border: oklch(0.922 0 0);
  --sidebar-ring: oklch(0.708 0 0);
}

.dark {
  --background: oklch(0.145 0 0);
  --foreground: oklch(0.985 0 0);
  --card: oklch(0.205 0 0);
  --card-foreground: oklch(0.985 0 0);
  --popover: oklch(0.205 0 0);
  --popover-foreground: oklch(0.985 0 0);
  --primary: oklch(0.922 0 0);
  --primary-foreground: oklch(0.205 0 0);
  --secondary: oklch(0.269 0 0);
  --secondary-foreground: oklch(0.985 0 0);
  --muted: oklch(0.269 0 0);
  --muted-foreground: oklch(0.708 0 0);
  --accent: oklch(0.269 0 0);
  --accent-foreground: oklch(0.985 0 0);
  --destructive: oklch(0.704 0.191 22.216);
  --border: oklch(1 0 0 / 10%);
  --input: oklch(1 0 0 / 15%);
  --ring: oklch(0.556 0 0);
  --chart-1: oklch(0.488 0.243 264.376);
  --chart-2: oklch(0.696 0.17 162.48);
  --chart-3: oklch(0.769 0.188 70.08);
  --chart-4: oklch(0.627 0.265 303.9);
  --chart-5: oklch(0.645 0.246 16.439);
  --sidebar: oklch(0.205 0 0);
  --sidebar-foreground: oklch(0.985 0 0);
  --sidebar-primary: oklch(0.488 0.243 264.376);
  --sidebar-primary-foreground: oklch(0.985 0 0);
  --sidebar-accent: oklch(0.269 0 0);
  --sidebar-accent-foreground: oklch(0.985 0 0);
  --sidebar-border: oklch(1 0 0 / 10%);
  --sidebar-ring: oklch(0.556 0 0);
}
```

The Tailwind `@theme inline` block that accompanies it (Tailwind v4 emits these as `:root`
custom properties; alder-ui writes them out by hand):

```css
--radius-sm: calc(var(--radius) * 0.6);
--radius-md: calc(var(--radius) * 0.8);
--radius-lg: var(--radius);
--radius-xl: calc(var(--radius) * 1.4);
--radius-2xl: calc(var(--radius) * 1.8);
```

(The older `create-next-app` template used `calc(var(--radius) - 4px)`, `- 2px`, `var(--radius)`,
`+ 4px`; at 0.625rem both scales give 6 / 8 / 10 / 14 px.)

`@layer base { * { @apply border-border outline-ring/50 } body { @apply bg-background text-foreground } }`.

The docs site's `globals.css` differs from the canonical sheet: `--foreground`/`--primary`
`oklch(0 0 0)`, dark `--accent oklch(0.371 0 0)`, `--destructive-foreground` added
(light `oklch(0.97 0.01 17)`, dark `oklch(0.58 0.22 27)`), `--surface`, `--code-*`,
`--selection`, and chart colours from Tailwind's blue scale. The design spec picks the
canonical sheet and keeps `--destructive-foreground`.

Font: the registry is font-agnostic. The docs site and templates load **Geist** (sans and
headings) and **Geist Mono** with `next/font`; body `font-size` is the browser's 16px;
components set `text-sm` themselves. `body { font-synthesis-weight: none; text-rendering:
optimizeLegibility }` on the docs site.

## 2. Tailwind v4 values the recipes resolve to

| class | value |
|---|---|
| `shadow-xs` | `0 1px 2px 0 rgb(0 0 0 / 0.05)` |
| `shadow-sm` | `0 1px 3px 0 rgb(0 0 0 / 0.1), 0 1px 2px -1px rgb(0 0 0 / 0.1)` |
| `shadow-md` | `0 4px 6px -1px rgb(0 0 0 / 0.1), 0 2px 4px -2px rgb(0 0 0 / 0.1)` |
| `shadow-lg` | `0 10px 15px -3px rgb(0 0 0 / 0.1), 0 4px 6px -4px rgb(0 0 0 / 0.1)` |
| `text-xs` / `text-sm` / `text-base` / `text-lg` | 0.75rem/1rem; 0.875rem/1.25rem; 1rem/1.5rem; 1.125rem/1.75rem |
| `text-xl` / `2xl` / `3xl` / `4xl` | 1.25rem/1.75rem; 1.5rem/2rem; 1.875rem/2.25rem; 2.25rem/2.5rem |
| spacing unit | 0.25rem: `h-8` 2rem, `h-9` 2.25rem, `h-10` 2.5rem, `px-2` 0.5rem, `px-3` 0.75rem, `px-4` 1rem, `px-6` 1.5rem, `gap-1.5` 0.375rem, `gap-2` 0.5rem, `size-4` 1rem, `p-[3px]` 3px |
| `rounded-sm/md/lg/xl` | `var(--radius-sm/md/lg/xl)`; `rounded-full` 9999px; `rounded-[4px]` 4px; `rounded-xs` 0.125rem |
| `font-medium` / `semibold` / `extrabold` | 500 / 600 / 800; `tracking-tight` -0.025em; `tracking-widest` 0.1em; `leading-none` 1; `leading-7` 1.75rem; `leading-relaxed` 1.625 |
| `bg-x/NN` | `color-mix(in oklab, var(--x) NN%, transparent)` |
| `ring-[3px] ring-ring/50` | `box-shadow: 0 0 0 3px color-mix(in oklab, var(--ring) 50%, transparent)` |
| `focus-visible:border-ring` | `border-color: var(--ring)` |
| `aria-invalid:*` | `border-color: var(--destructive); box-shadow: 0 0 0 3px destructive/20` (dark `/40`) |
| `disabled:*` | `opacity: .5; pointer-events: none` (inputs: `cursor: not-allowed`) |
| `[&_svg:not([class*='size-'])]:size-4` | child `svg` 1rem square, `flex: none`, `pointer-events: none` |
| `transition-all` / `transition-[color,box-shadow]` | 150ms, `cubic-bezier(.4,0,.2,1)` |
| `animate-in fade-in-0 zoom-in-95` | enter 150ms (dialog `duration-200`): opacity 0→1, scale .95→1; `slide-in-from-top-2` translateY -0.5rem→0 (and the other sides); `animate-out …` mirrors. Not fetched verbatim from tw-animate-css; treat as the intent, not the exact curve. |

## 3. Component recipes (verbatim class strings)

### button

Base:
`inline-flex shrink-0 items-center justify-center gap-2 rounded-md text-sm font-medium whitespace-nowrap transition-all outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:pointer-events-none disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4`

| variant | classes |
|---|---|
| default | `bg-primary text-primary-foreground hover:bg-primary/90` |
| destructive | `bg-destructive text-white hover:bg-destructive/90 focus-visible:ring-destructive/20 dark:bg-destructive/60 dark:focus-visible:ring-destructive/40` |
| outline | `border bg-background shadow-xs hover:bg-accent hover:text-accent-foreground dark:border-input dark:bg-input/30 dark:hover:bg-input/50` |
| secondary | `bg-secondary text-secondary-foreground hover:bg-secondary/80` |
| ghost | `hover:bg-accent hover:text-accent-foreground dark:hover:bg-accent/50` |
| link | `text-primary underline-offset-4 hover:underline` |

| size | classes |
|---|---|
| default | `h-9 px-4 py-2 has-[>svg]:px-3` |
| xs | `h-6 gap-1 rounded-md px-2 text-xs has-[>svg]:px-1.5 [&_svg:not([class*='size-'])]:size-3` |
| sm | `h-8 gap-1.5 rounded-md px-3 has-[>svg]:px-2.5` |
| lg | `h-10 rounded-md px-6 has-[>svg]:px-4` |
| icon | `size-9`; icon-xs `size-6 rounded-md [&_svg:not([class*='size-'])]:size-3`; icon-sm `size-8`; icon-lg `size-10` |

### input

`h-9 w-full min-w-0 rounded-md border border-input bg-transparent px-3 py-1 text-base shadow-xs transition-[color,box-shadow] outline-none selection:bg-primary selection:text-primary-foreground file:inline-flex file:h-7 file:border-0 file:bg-transparent file:text-sm file:font-medium file:text-foreground placeholder:text-muted-foreground disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 md:text-sm dark:bg-input/30`
+ `focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50`
+ `aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40`

### textarea

`flex field-sizing-content min-h-16 w-full rounded-md border border-input bg-transparent px-3 py-2 text-base shadow-xs transition-[color,box-shadow] outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 md:text-sm dark:bg-input/30 dark:aria-invalid:ring-destructive/40`

### label

`flex items-center gap-2 text-sm leading-none font-medium select-none group-data-[disabled=true]:pointer-events-none group-data-[disabled=true]:opacity-50 peer-disabled:cursor-not-allowed peer-disabled:opacity-50`

Form field composition in the shadcn demos: `<div class="grid gap-2">` (or `gap-3`) holding
Label, Input, then `<p class="text-sm text-muted-foreground">` (description) or
`<p class="text-sm text-destructive">` (error message, `font-medium` in `FormMessage`).

### card

| part | classes |
|---|---|
| Card | `flex flex-col gap-6 rounded-xl border bg-card py-6 text-card-foreground shadow-sm` |
| CardHeader | `@container/card-header grid auto-rows-min grid-rows-[auto_auto] items-start gap-2 px-6 has-data-[slot=card-action]:grid-cols-[1fr_auto] [.border-b]:pb-6` |
| CardTitle | `leading-none font-semibold` |
| CardDescription | `text-sm text-muted-foreground` |
| CardAction | `col-start-2 row-span-2 row-start-1 self-start justify-self-end` |
| CardContent | `px-6` |
| CardFooter | `flex items-center px-6 [.border-t]:pt-6` |

### badge

Base: `inline-flex w-fit shrink-0 items-center justify-center gap-1 overflow-hidden rounded-full border border-transparent px-2 py-0.5 text-xs font-medium whitespace-nowrap transition-[color,box-shadow] focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 [&>svg]:pointer-events-none [&>svg]:size-3`

| variant | classes |
|---|---|
| default | `bg-primary text-primary-foreground [a&]:hover:bg-primary/90` |
| secondary | `bg-secondary text-secondary-foreground [a&]:hover:bg-secondary/90` |
| destructive | `bg-destructive text-white focus-visible:ring-destructive/20 dark:bg-destructive/60 dark:focus-visible:ring-destructive/40 [a&]:hover:bg-destructive/90` |
| outline | `border-border text-foreground [a&]:hover:bg-accent [a&]:hover:text-accent-foreground` |
| ghost | `[a&]:hover:bg-accent [a&]:hover:text-accent-foreground` |
| link | `text-primary underline-offset-4 [a&]:hover:underline` |

### alert

Base: `relative grid w-full grid-cols-[0_1fr] items-start gap-y-0.5 rounded-lg border px-4 py-3 text-sm has-[>svg]:grid-cols-[calc(var(--spacing)*4)_1fr] has-[>svg]:gap-x-3 [&>svg]:size-4 [&>svg]:translate-y-0.5 [&>svg]:text-current`

| variant | classes |
|---|---|
| default | `bg-card text-card-foreground` |
| destructive | `bg-card text-destructive *:data-[slot=alert-description]:text-destructive/90 [&>svg]:text-current` |

AlertTitle `col-start-2 line-clamp-1 min-h-4 font-medium tracking-tight`;
AlertDescription `col-start-2 grid justify-items-start gap-1 text-sm text-muted-foreground [&_p]:leading-relaxed`.
shadcn has no info/success/warning variants and no tinted backgrounds.

### checkbox

Root: `peer size-4 shrink-0 rounded-[4px] border border-input shadow-xs transition-shadow outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 data-[state=checked]:border-primary data-[state=checked]:bg-primary data-[state=checked]:text-primary-foreground dark:bg-input/30 dark:aria-invalid:ring-destructive/40 dark:data-[state=checked]:bg-primary`.
Indicator: `flex items-center justify-center text-current transition-none` with `<CheckIcon className="size-3.5" />`.

> **Amended 2026-10-24 (alder-ui `ref/`, `shadcn@4.22.1 add checkbox`, new-york-v4):** the
> Indicator is now `grid place-content-center text-current transition-none` (same
> centring, grid instead of flex); the root string above is unchanged.

### switch (deferred; for the record)

Root: `peer group/switch inline-flex shrink-0 items-center rounded-full border border-transparent shadow-xs transition-all outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 data-[size=default]:h-[1.15rem] data-[size=default]:w-8 data-[size=sm]:h-3.5 data-[size=sm]:w-6 data-[state=checked]:bg-primary data-[state=unchecked]:bg-input dark:data-[state=unchecked]:bg-input/80`.
Thumb: `pointer-events-none block rounded-full bg-background ring-0 transition-transform group-data-[size=default]/switch:size-4 group-data-[size=sm]/switch:size-3 data-[state=checked]:translate-x-[calc(100%-2px)] data-[state=unchecked]:translate-x-0 dark:data-[state=checked]:bg-primary-foreground dark:data-[state=unchecked]:bg-foreground`.

### radio-group (deferred; for the record)

Group `grid gap-3`. Item: `aspect-square size-4 shrink-0 rounded-full border border-input text-primary shadow-xs transition-[color,box-shadow] outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:bg-input/30 dark:aria-invalid:ring-destructive/40`; indicator a `size-2 fill-primary` circle centred.

### select

Trigger: `flex w-fit items-center justify-between gap-2 rounded-md border border-input bg-transparent px-3 py-2 text-sm whitespace-nowrap shadow-xs transition-[color,box-shadow] outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 data-[placeholder]:text-muted-foreground data-[size=default]:h-9 data-[size=sm]:h-8 *:data-[slot=select-value]:line-clamp-1 *:data-[slot=select-value]:flex *:data-[slot=select-value]:items-center *:data-[slot=select-value]:gap-2 dark:bg-input/30 dark:hover:bg-input/50 dark:aria-invalid:ring-destructive/40 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4 [&_svg:not([class*='text-'])]:text-muted-foreground` with `<ChevronDownIcon className="size-4 opacity-50" />`.
Content: `relative z-50 max-h-(--radix-select-content-available-height) min-w-[8rem] origin-(--radix-select-content-transform-origin) overflow-x-hidden overflow-y-auto rounded-md border bg-popover text-popover-foreground shadow-md` + animations; viewport `p-1`.
Label `px-2 py-1.5 text-xs text-muted-foreground`.
Item: `relative flex w-full cursor-default items-center gap-2 rounded-sm py-1.5 pr-8 pl-2 text-sm outline-hidden select-none focus:bg-accent focus:text-accent-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50 …` with a `size-4` check at `right-2`.
Separator `pointer-events-none -mx-1 my-1 h-px bg-border`.

### dialog

| part | classes |
|---|---|
| Overlay | `fixed inset-0 z-50 bg-black/50 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:animate-in data-[state=open]:fade-in-0` |
| Content | `fixed top-[50%] left-[50%] z-50 grid w-full max-w-[calc(100%-2rem)] translate-x-[-50%] translate-y-[-50%] gap-4 rounded-lg border bg-background p-6 shadow-lg duration-200 outline-none data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95 data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95 sm:max-w-lg` |
| Close | `absolute top-4 right-4 rounded-xs opacity-70 ring-offset-background transition-opacity hover:opacity-100 focus:ring-2 focus:ring-ring focus:ring-offset-2 focus:outline-hidden disabled:pointer-events-none data-[state=open]:bg-accent data-[state=open]:text-muted-foreground [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4` with `<XIcon />` and `sr-only` "Close" |
| Header | `flex flex-col gap-2 text-center sm:text-left` |
| Footer | `flex flex-col-reverse gap-2 sm:flex-row sm:justify-end` |
| Title | `text-lg leading-none font-semibold` |
| Description | `text-sm text-muted-foreground` |

### dropdown-menu

| part | classes |
|---|---|
| Content | `z-50 max-h-(--radix-dropdown-menu-content-available-height) min-w-[8rem] origin-(--radix-dropdown-menu-content-transform-origin) overflow-x-hidden overflow-y-auto rounded-md border bg-popover p-1 text-popover-foreground shadow-md data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95 data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95` (`sideOffset={4}`) |
| Item | `relative flex cursor-default items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-hidden select-none focus:bg-accent focus:text-accent-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50 data-[inset]:pl-8 data-[variant=destructive]:text-destructive data-[variant=destructive]:focus:bg-destructive/10 data-[variant=destructive]:focus:text-destructive dark:data-[variant=destructive]:focus:bg-destructive/20 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4 [&_svg:not([class*='text-'])]:text-muted-foreground` |
| Checkbox/Radio item | `relative flex cursor-default items-center gap-2 rounded-sm py-1.5 pr-2 pl-8 text-sm …` indicator at `left-2 size-3.5` |
| Label | `px-2 py-1.5 text-sm font-medium data-[inset]:pl-8` |
| Separator | `-mx-1 my-1 h-px bg-border` |
| Shortcut | `ml-auto text-xs tracking-widest text-muted-foreground` |
| SubTrigger | `flex cursor-default items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-hidden select-none focus:bg-accent focus:text-accent-foreground data-[inset]:pl-8 data-[state=open]:bg-accent data-[state=open]:text-accent-foreground …` + `<ChevronRightIcon className="ml-auto size-4" />` |
| SubContent | `z-50 min-w-[8rem] … overflow-hidden rounded-md border bg-popover p-1 text-popover-foreground shadow-lg …` |

> **Note 2026-10-24 (alder-ui Task 15, checked against `ref/src/components/ui/dropdown-menu.tsx`):**
> the Item string above omits its last class, `data-[variant=destructive]:*:[svg]:text-destructive!`:
> an svg that is a direct child of a destructive item takes `text-destructive`, overriding the
> `text-muted-foreground` every other item svg gets. `al-menu` implements it.

### popover

Content: `z-50 w-72 origin-(--radix-popover-content-transform-origin) rounded-md border bg-popover p-4 text-popover-foreground shadow-md outline-hidden` + the side/state animations (`sideOffset={4}`). PopoverHeader `flex flex-col gap-1 text-sm`; Title `font-medium`; Description `text-muted-foreground`.

### tabs

Root: `group/tabs flex gap-2 data-[orientation=horizontal]:flex-col data-[orientation=vertical]:flex-row`.
List: `group/tabs-list inline-flex w-fit items-center justify-center rounded-lg p-[3px] text-muted-foreground group-data-[orientation=horizontal]/tabs:h-9 group-data-[orientation=vertical]/tabs:h-fit group-data-[orientation=vertical]/tabs:flex-col data-[variant=line]:rounded-none`; variant default `bg-muted`, line `gap-1 bg-transparent`.
Trigger: `relative inline-flex h-[calc(100%-1px)] flex-1 items-center justify-center gap-1.5 rounded-md border border-transparent px-2 py-1 text-sm font-medium whitespace-nowrap text-foreground/60 transition-all group-data-[orientation=vertical]/tabs:w-full group-data-[orientation=vertical]/tabs:justify-start hover:text-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-1 focus-visible:outline-ring disabled:pointer-events-none disabled:opacity-50 group-data-[variant=default]/tabs-list:data-[state=active]:shadow-sm group-data-[variant=line]/tabs-list:data-[state=active]:shadow-none dark:text-muted-foreground dark:hover:text-foreground [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4`,
plus (default variant, active) `bg-background text-foreground` and dark `bg-input/30 border-input text-foreground`; (line variant, active) an `after:` 2px `bg-foreground` underline at the bottom (`after:absolute after:inset-x-0 after:-bottom-[4px] after:h-0.5 after:bg-foreground`) — the builder should re-fetch `tabs.tsx` for the exact `line` classes, which were not copied in full.
Content: `flex-1 outline-none`.

> **Amended 2026-10-24 (alder-ui `ref/`, re-fetched `tabs.tsx` via `shadcn@4.22.1 add tabs`,
> new-york-v4):**
> - Root is `group/tabs flex gap-2 data-[orientation=horizontal]:flex-col` (no
>   `data-[orientation=vertical]:flex-row`); `Tabs` also sets `data-orientation`.
> - List and the first Trigger string are unchanged. The Trigger's `cn()` carries three
>   more strings, verbatim:
>   1. line variant: `group-data-[variant=line]/tabs-list:bg-transparent group-data-[variant=line]/tabs-list:data-[state=active]:bg-transparent dark:group-data-[variant=line]/tabs-list:data-[state=active]:border-transparent dark:group-data-[variant=line]/tabs-list:data-[state=active]:bg-transparent`
>   2. active: `data-[state=active]:bg-background data-[state=active]:text-foreground dark:data-[state=active]:border-input dark:data-[state=active]:bg-input/30 dark:data-[state=active]:text-foreground`
>   3. underline: `after:absolute after:bg-foreground after:opacity-0 after:transition-opacity group-data-[orientation=horizontal]/tabs:after:inset-x-0 group-data-[orientation=horizontal]/tabs:after:bottom-[-5px] group-data-[orientation=horizontal]/tabs:after:h-0.5 group-data-[orientation=vertical]/tabs:after:inset-y-0 group-data-[orientation=vertical]/tabs:after:-right-1 group-data-[orientation=vertical]/tabs:after:w-0.5 group-data-[variant=line]/tabs-list:data-[state=active]:after:opacity-100`
> - So the line underline sits at `bottom: -5px` (not `-4px`), is 2px tall, spans the
>   trigger (`inset-x-0`), exists on every trigger at `opacity: 0` and fades to `1` on the
>   active one (`transition-opacity`, 150ms); in the line variant the active trigger keeps
>   a transparent background (and a transparent border in dark), and `shadow-none`.

### table

| part | classes |
|---|---|
| wrapper | `relative w-full overflow-x-auto` |
| Table | `w-full caption-bottom text-sm` |
| TableHeader | `[&_tr]:border-b` |
| TableBody | `[&_tr:last-child]:border-0` |
| TableFooter | `border-t bg-muted/50 font-medium [&>tr]:last:border-b-0` |
| TableRow | `border-b transition-colors hover:bg-muted/50 has-aria-expanded:bg-muted/50 data-[state=selected]:bg-muted` |
| TableHead | `h-10 px-2 text-left align-middle font-medium whitespace-nowrap text-foreground [&:has([role=checkbox])]:pr-0 [&>[role=checkbox]]:translate-y-[2px]` |
| TableCell | `p-2 align-middle whitespace-nowrap [&:has([role=checkbox])]:pr-0 [&>[role=checkbox]]:translate-y-[2px]` |
| TableCaption | `mt-4 text-sm text-muted-foreground` |

### separator

`shrink-0 bg-border data-[orientation=horizontal]:h-px data-[orientation=horizontal]:w-full data-[orientation=vertical]:h-full data-[orientation=vertical]:w-px`

### tooltip

Content: `z-50 w-fit origin-(--radix-tooltip-content-transform-origin) animate-in rounded-md bg-foreground px-3 py-1.5 text-xs text-balance text-background fade-in-0 zoom-in-95 data-[side=bottom]:slide-in-from-top-2 … data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95` (`sideOffset={0}`).
Arrow: `z-50 size-2.5 translate-y-[calc(-50%_-_2px)] rotate-45 rounded-[2px] bg-foreground fill-foreground`.

### sonner (toast)

The Toaster sets `--normal-bg: var(--popover); --normal-text: var(--popover-foreground); --normal-border: var(--border); --border-radius: var(--radius)` and icons `size-4`: success CircleCheck, info Info, warning TriangleAlert, error OctagonX, loading Loader2 (spinning). Sonner's own toast: width 356px, `padding: 16px`, `gap: 6px` between icon/title, `box-shadow` `0 4px 12px rgb(0 0 0 / .1)`, title `font-weight 500; line-height 1.5; font-size 13px`, description `font-weight 400; color: inherit; opacity .8`, bottom-right with 32px offset and 14px gap between toasts.

> **Amended 2026-10-24 (alder-ui `ref/`, `sonner@2.0.8` as installed by `shadcn@4.22.1 add
> sonner`):** `sonner.tsx` matches the above. Sonner's own stylesheet differs in places
> from the summary: the viewport offset is **24px** (`VIEWPORT_OFFSET`; 16px on mobile),
> not 32px; the toaster sets its own `font-family: ui-sans-serif, system-ui, …` (so toasts
> do not inherit Geist unless the app overrides it); the description is `font-weight: 400;
> line-height: 1.4; color: #3f3f3f` (dark `hsl(0, 0%, 91%)`), not `inherit` at 0.8 opacity;
> the title is `font-weight: 500; line-height: 1.5; color: inherit`; toasts enter with a
> 400ms transform/opacity transition. Width 356px, gap 14px, padding 16px, border
> `var(--normal-border)`, radius `var(--border-radius)` and shadow `0 4px 12px rgb(0 0 0 / .1)`
> are as stated.

### skeleton (deferred)

`bg-accent animate-pulse rounded-md`.

### pagination

Pagination `mx-auto flex w-full justify-center`; Content `flex flex-row items-center gap-1`; Link = `buttonVariants({ variant: isActive ? "outline" : "ghost", size })` (`size` default `icon`), `aria-current="page"` on the active one; Previous `gap-1 px-2.5 sm:pl-2.5` with `<ChevronLeftIcon />` and `<span class="hidden sm:block">Previous</span>`; Next mirrors with `ChevronRightIcon`; Ellipsis `flex size-9 items-center justify-center` with `<MoreHorizontalIcon className="size-4" />`.

### typography (the docs' Typography page, used for raw flow content)

`h1 scroll-m-20 text-4xl font-extrabold tracking-tight text-balance`;
`h2 scroll-m-20 border-b pb-2 text-3xl font-semibold tracking-tight first:mt-0` (`mt-10` between sections);
`h3 scroll-m-20 text-2xl font-semibold tracking-tight`; `h4 scroll-m-20 text-xl font-semibold tracking-tight`;
`p leading-7 [&:not(:first-child)]:mt-6`; `blockquote mt-6 border-l-2 pl-6 italic`;
`ul my-6 ml-6 list-disc [&>li]:mt-2`; `code relative rounded bg-muted px-[0.3rem] py-[0.2rem] font-mono text-sm font-semibold`;
`a font-medium text-primary underline underline-offset-4`; lead `text-muted-foreground text-xl`; large `text-lg font-semibold`; small `text-sm leading-none font-medium`; muted `text-muted-foreground text-sm`;
table: `my-6 w-full overflow-y-auto`, `tr m-0 border-t p-0 even:bg-muted`, `th border px-4 py-2 text-left font-bold`, `td border px-4 py-2 text-left`.

## 4. Lucide icons the restyle needs

check, minus, chevron-down, chevron-left, chevron-right, x, circle-check, info,
triangle-alert, octagon-x, loader-circle, ellipsis. lucide is ISC-licensed
(`https://github.com/lucide-icons/lucide`); the SVGs are 24×24, `stroke="currentColor"
stroke-width="2" stroke-linecap="round" stroke-linejoin="round" fill="none"`.
