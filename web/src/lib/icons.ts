import { COMMON_ICONS, registerIcons } from '@syware/jlib';
import {
  ArrowUp,
  Bell,
  BellOff,
  BookOpen,
  BookPlus,
  Brain,
  Camera,
  Check,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  Circle,
  CircleAlert,
  CircleArrowUp,
  CircleQuestionMark,
  Copy,
  CornerLeftUp,
  Cpu,
  FilePlus,
  FileText,
  Folder,
  FolderPlus,
  FolderSearch,
  FoldVertical,
  Gamepad2,
  Globe,
  Image,
  Keyboard,
  KeyRound,
  Laptop,
  Link,
  ListChecks,
  LoaderCircle,
  Mic,
  Moon,
  Network,
  PanelRightClose,
  PanelRightOpen,
  Pause,
  Pencil,
  Plus,
  Power,
  RefreshCw,
  Rocket,
  Search,
  Server,
  Settings,
  ShieldCheck,
  SlidersHorizontal,
  Smartphone,
  Split,
  Square,
  SquarePen,
  Sun,
  Terminal,
  Trash2,
  X,
} from 'lucide';

import type { IconNode } from 'lucide';

// The pace of jlib's own spinner (`jl-spinner`), slower for people who ask for less motion.
const TURN_SECONDS = { calm: 1.6, usual: 0.7 } as const;
const REDUCED_MOTION = '(prefers-reduced-motion: reduce)';
// lucide's grid is 24 units wide: its icons turn about 12, 12.
const TURN = {
  attributeName: 'transform',
  from: '0 12 12',
  repeatCount: 'indefinite',
  to: '360 12 12',
  type: 'rotate',
} as const;

/**
 * An icon whose shapes turn on their own. jlib draws icons inside its components, where no stylesheet of the app
 * reaches, so the turn is the SVG's own animation. lucide draws a node's third item as the nodes inside it, though
 * its type knows only two.
 */
const turning = (icon: IconNode): IconNode => {
  const seconds = matchMedia(REDUCED_MOTION).matches ? TURN_SECONDS.calm : TURN_SECONDS.usual;
  return icon.map(([tag, attrs]) => {
    const node: IconNode[number] = [tag, attrs];
    return Object.assign(node, { 2: [['animateTransform', { ...TURN, dur: `${seconds}s` }]] });
  });
};

const ICONS = {
  'arrow-up': ArrowUp,
  bell: Bell,
  'bell-off': BellOff,
  'book-open': BookOpen,
  'book-plus': BookPlus,
  brain: Brain,
  camera: Camera,
  check: Check,
  'chevron-down': ChevronDown,
  'chevron-left': ChevronLeft,
  'chevron-right': ChevronRight,
  circle: Circle,
  'circle-alert': CircleAlert,
  // A newer version of swcode, to install.
  'circle-arrow-up': CircleArrowUp,
  'circle-question': CircleQuestionMark,
  copy: Copy,
  'corner-left-up': CornerLeftUp,
  cpu: Cpu,
  'file-plus': FilePlus,
  'file-text': FileText,
  'fold-vertical': FoldVertical,
  folder: Folder,
  'folder-plus': FolderPlus,
  'folder-search': FolderSearch,
  gamepad: Gamepad2,
  globe: Globe,
  image: Image,
  'key-round': KeyRound,
  keyboard: Keyboard,
  laptop: Laptop,
  link: Link,
  'list-checks': ListChecks,
  // What runs (a session's turn, a step of the plan) shows it.
  loader: turning(LoaderCircle),
  mic: Mic,
  moon: Moon,
  network: Network,
  'panel-close': PanelRightClose,
  'panel-open': PanelRightOpen,
  pause: Pause,
  pencil: Pencil,
  plus: Plus,
  power: Power,
  refresh: RefreshCw,
  rocket: Rocket,
  search: Search,
  server: Server,
  settings: Settings,
  shield: ShieldCheck,
  'sliders-horizontal': SlidersHorizontal,
  smartphone: Smartphone,
  split: Split,
  'square-pen': SquarePen,
  stop: Square,
  sun: Sun,
  terminal: Terminal,
  trash: Trash2,
  x: X,
} satisfies Record<string, IconNode>;

export type IconName = keyof typeof ICONS;

// jlib's components draw icons by name: its usual ones, then swcode's own (`shield`, `stop`, `panel-open`…), which
// take the names they share (`loader`, which turns here).
registerIcons(COMMON_ICONS);
registerIcons(ICONS);
