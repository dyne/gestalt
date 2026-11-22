import { Box, Panel, Text, component, render, useApp, useInteraction, useShortcut, useState,
  useTerminalSize } from 'tuiuiu.js';
import type { AppHandle, ComponentKeyProps, ScreenMode } from 'tuiuiu.js';
import { PromptCancelledError } from 'tuiuiu.js/interaction';
import { profileName, reconcileSkills } from './model.ts';
import type { CatalogSkill, SkillEntry, SkillProfile } from './model.ts';

export interface ProfileList { readonly profiles: SkillProfile[]; readonly warnings: string[] }
export interface Catalog { readonly skills: CatalogSkill[]; readonly warnings: string[] }
export interface EditorPorts {
  list(): Promise<ProfileList>;
  discover(): Promise<Catalog>;
  defaults(): Promise<SkillProfile | undefined>;
  save(profile: SkillProfile): Promise<void>;
  delete(name: string): Promise<boolean>;
}

const accent = '#c7460c';
const border = '#75695b';
const safeText = (value: string) => value.replace(/[\u0000-\u001f\u007f-\u009f]/g, ' ');
const message = (error: unknown) => safeText(error instanceof Error ? error.message : String(error));
const instructions = (text: string) => Text({ dim: true, wrap: 'wrap' }, text);

/** Hand the original terminal back in paused mode before Codex borrows its input. */
async function runTui(view: Parameters<typeof render>[0], screen: ScreenMode, mounted?: (app: AppHandle) => void): Promise<void> {
  process.stdin.pause();
  const app = render(view, { screen, autoTabNavigation: false });
  mounted?.(app);
  try { await app.waitUntilExit(); }
  finally { process.stdin.pause(); }
}

export type PickerChoice = { kind: 'none' | 'manage' } | { kind: 'profile'; profile: SkillProfile };

type MenuRow = { value: string; label: string; group?: string; color?: string; disabled?: boolean; description?: string };
type MenuProps = ComponentKeyProps & { rows: readonly MenuRow[]; limit: number; enabled?: boolean; initial?: string;
  submit(value: string): void; cancel(): void };

/** A windowed menu whose Enter action follows the highlighted row. */
const ProfileMenu = component('ProfileMenu', ({ rows, limit, enabled = true, initial, submit, cancel }: MenuProps) => {
  const [highlight, setHighlight] = useState(initial);
  const activeRow = () => rows.find(row => row.value === highlight() && !row.disabled) ?? rows.find(row => !row.disabled);
  const active = activeRow();
  const index = active ? rows.indexOf(active) : 0;
  function move(direction: -1 | 1): void {
    const current = activeRow(), start = current ? rows.indexOf(current) : 0;
    for (let next = start + direction; next >= 0 && next < rows.length; next += direction) {
      const row = rows[next]!;
      if (!row.disabled) { setHighlight(row.value); return; }
    }
  }
  useShortcut(['up', 'k'], () => move(-1), { isActive: enabled });
  useShortcut(['down', 'j'], () => move(1), { isActive: enabled });
  useShortcut(['enter', 'space'], () => { const current = activeRow(); if (current) submit(current.value); }, { isActive: enabled });
  useInteraction(event => {
    if (event.type === 'key' && event.key.native.escape) { cancel(); return true; }
    return false;
  }, undefined, enabled);
  const start = Math.max(0, Math.min(index - Math.floor(limit / 2), rows.length - limit));
  const visible = rows.slice(start, start + limit);
  return Box({ flexDirection: 'column' }, ...visible.flatMap((row, offset) => [
    ...(row.group && (offset === 0 || row.group !== visible[offset - 1]?.group)
      ? [Text({ color: accent, dim: true }, row.group)] : []),
    Text({ ...(row.color ? { color: row.color } : row === active ? { color: accent } : {}),
      bold: row === active, dim: row.disabled ?? false, wrap: 'truncate' }, row === active ? '› ' : '  ', row.label),
    ...(row === active && row.description ? [Text({ dim: true, wrap: 'truncate' }, `  ${row.description}`)] : []),
  ]), ...(rows.length > limit ? [instructions(`${index + 1} of ${rows.length}`)] : []));
});

/** Choose a launch-only profile. Leaving the picker cancels the launch. */
export async function pickProfile(list: ProfileList): Promise<PickerChoice | undefined> {
  let choice: PickerChoice | undefined;
  const choices = new Map<string, PickerChoice>([
    ...list.profiles.map(profile => [profile.name, { kind: 'profile', profile }] as const),
    [':manage', { kind: 'manage' }], [':none', { kind: 'none' }]]);
  const Picker = component('SkillProfilePicker', () => {
    const { exit } = useApp();
    const { rows } = useTerminalSize();
    return Panel({ title: 'Choose a skill profile', borderStyle: 'round', borderColor: border, paddingX: 1 },
      ...list.warnings.slice(0, 2).map(warning => Text({ color: 'yellow', wrap: 'truncate' }, safeText(warning))),
      ProfileMenu({ key: 'profiles', limit: Math.max(3, rows - 11), ...(!list.profiles.length ? { initial: ':none' } : {}),
        rows: [...choices.keys()].map(value => ({ value, label: value === ':none'
          ? 'None'
          : value === ':manage' ? 'Manage skill profiles…' : value })),
        submit: value => { choice = choices.get(value); exit(); }, cancel: () => exit(),
      }), instructions('↑ ↓  Select    Enter  Continue    Esc  Cancel'));
  });
  await runTui(() => Picker({ key: 'picker' }), 'inline');
  return choice;
}

type Draft = {
  selected: string;
  empty: boolean;
  name: string;
  skills: ReturnType<typeof reconcileSkills>['skills'];
  missing: SkillEntry[];
  browser: boolean | undefined;
};

function draftFor(catalog: Catalog, profile?: SkillProfile, selected = '', empty = false): Draft {
  return { selected, empty, name: selected, ...reconcileSkills(catalog.skills, empty ? [] : profile?.skills), browser: empty ? undefined : profile?.browser };
}

/** Edit the complete skill selection with the same fields and actions as Mobile. */
export async function editProfiles(ports: EditorPorts, screen: ScreenMode = 'alternate'): Promise<void> {
  let [list, catalog, defaults] = await Promise.all([ports.list(), ports.discover(), ports.defaults()]);
  let app: AppHandle;
  const Editor = component('SkillProfileEditor', () => {
    const { exit } = useApp();
    const { columns, rows } = useTerminalSize();
    const [draft, setDraft] = useState(draftFor(catalog, defaults));
    const [status, setStatus] = useState('');
    const [detail, setDetail] = useState<CatalogSkill | undefined>(undefined);
    const [busy, setBusy] = useState(false);
    const [dialog, setDialog] = useState<{ title: string; choices: readonly string[]; initial: string;
      resolve(value: string | undefined): void } | undefined>(undefined);
    const choose = (title: string, choices: readonly string[], initial = choices[0]!) => new Promise<string | undefined>(resolve => {
      setDialog({ title, choices, initial, resolve(value) { setDialog(undefined); resolve(value); } });
    });
    const confirm = async (title: string) => await choose(title, ['Cancel', 'Confirm']) === 'Confirm';
    const current = draft();
    const replacing = list.profiles.some(profile => profile.name === current.name);
    async function action(value: string) {
      if (busy()) return;
      setBusy(true);
      const active = draft();
      try {
        switch (value) {
          case 'profile': {
            const choice = await choose('Skill profile', ['New profile', ...list.profiles.map(profile => profile.name), 'None'],
              active.selected || (active.empty ? 'None' : 'New profile'));
            if (!choice) return;
            const selected = list.profiles.find(profile => profile.name === choice);
            setDraft(draftFor(catalog, selected ?? defaults, selected?.name, choice === 'None'));
            setDetail(undefined);
            break;
          }
          case 'name': {
            const name = await app.prompts.input('Save as', { default: active.name, validate: value => {
              try { profileName(value); return true; } catch (error) { return message(error); }
            } });
            setDraft({ ...active, name: profileName(name) });
            break;
          }
          case 'browser': setDraft({ ...active, browser: !active.browser }); break;
          case 'save': {
            const name = profileName(active.name || await app.prompts.input('Save as'));
            if (name !== active.selected && list.profiles.some(profile => profile.name === name) &&
                !await confirm(`Replace ${name}?`)) return;
            await ports.save({ name, skills: [...active.skills, ...active.missing],
              ...(active.browser !== undefined ? { browser: active.browser } : {}) });
            list = await ports.list();
            setDraft({ ...active, selected: name, name, empty: false });
            setStatus(`Saved ${name}.`);
            break;
          }
          case 'delete':
            if (active.selected && await confirm(`Delete ${active.selected}? Existing sessions keep their skills.`)) {
              if (!await ports.delete(active.selected)) throw new Error('The saved skill profile disappeared.');
              list = await ports.list();
              setDraft(draftFor(catalog, defaults));
              setDetail(undefined);
              setStatus(`Deleted ${active.selected}.`);
            }
            break;
          case 'refresh': {
            const next = await ports.discover();
            const baseline = draftFor(catalog, defaults);
            const selection = active.selected || active.name || active.skills.some((skill, index) =>
              skill.enabled !== baseline.skills[index]?.enabled) || active.missing.length !== baseline.missing.length
              ? [...active.skills, ...active.missing] : active.empty ? [] : defaults?.skills;
            catalog = next;
            setDraft({ ...active, ...reconcileSkills(next.skills, selection) });
            setDetail(undefined);
            setStatus('Refreshed skills.');
            break;
          }
          default: {
            const skillIndex = Number(value.slice(2));
            if (value.startsWith('m:')) {
              setDraft({ ...active, missing: active.missing.filter((_skill, index) => index !== skillIndex) });
            } else {
              const skill = active.skills[skillIndex];
              if (!skill) return;
              setDetail(skill);
              const toggle = skill.enabled ? 'Disable skill' : 'Enable skill';
              if (await choose(safeText(skill.displayName ?? skill.name), [toggle, 'Back']) !== toggle) return;
              const updated = { ...skill, enabled: !skill.enabled };
              setDetail(updated);
              setDraft({ ...active, skills: active.skills.map((item, index) => index === skillIndex ? updated : item) });
            }
          }
        }
      } catch (error) {
        if (!(error instanceof PromptCancelledError)) setStatus(message(error));
      } finally { setBusy(false); }
    }
    useShortcut('ctrl+s', () => action('save'));
    useShortcut('ctrl+r', () => action('refresh'));
    const selected = detail();
    const warning = [...list.warnings, ...catalog.warnings][0];
    const currentDialog = dialog();
    const rowsBelow = selected && rows >= 20 ? 17 : 12;
    return Panel({ title: 'Manage skill profiles', borderStyle: 'round', borderColor: border, paddingX: 1 },
      Text({ color: accent }, `${current.skills.filter(skill => skill.enabled).length} of ${current.skills.length} skills enabled`),
      currentDialog ? Panel({ title: currentDialog.title, borderStyle: 'single', borderColor: accent, paddingX: 1 },
        ProfileMenu({ key: 'dialog', initial: currentDialog.initial, limit: Math.max(2, rows - 17),
          rows: currentDialog.choices.map(value => ({ value, label: value })),
          submit: value => currentDialog.resolve(value), cancel: () => currentDialog.resolve(undefined) }))
      : ProfileMenu({ key: 'editor-controls', enabled: !busy(), limit: Math.max(3, rows - rowsBelow),
        rows: [
          { value: 'profile', label: `Skill profile  ${current.selected || (current.empty ? 'None' : 'New profile')}`, group: 'Profile' },
          { value: 'name', label: `Save as        ${current.name || 'Enter a name'}` },
          { value: 'browser', label: `${current.browser ? '[x]' : '[ ]'} Browser access (Playwright)`,
            description: current.browser ? 'Configured · runtime availability depends on the host' : 'Off' },
          { value: 'save', label: replacing ? 'Save profile · replace' : 'Save profile · create' },
          { value: 'delete', label: 'Delete profile', disabled: !current.selected },
          { value: 'refresh', label: 'Refresh skills' },
          ...current.missing.map((skill, index) => ({ value: `m:${index}`, label: `Remove ${safeText(skill.name)} · missing`,
            group: 'Missing skills', color: 'yellow', description: safeText(skill.path) })),
          ...current.skills.map((skill, index) => ({ value: `s:${index}`, group: 'Skills',
            label: `${skill.enabled ? '[x]' : '[ ]'} ${safeText(skill.displayName ?? skill.name)}${skill.verified ? ' · Gestalt verified' : ''}`,
            ...(/^#[a-f0-9]{6}$/i.test(skill.brandColor ?? '') && columns > 60 ? { color: skill.brandColor } : {}),
          })),
        ],
        submit: value => { void action(value); }, cancel: () => exit(),
      }),
      ...(selected && rows >= 20 ? [Box({ flexDirection: 'column', borderStyle: 'single', borderColor: accent, paddingX: 1 },
        Text({ bold: true, wrap: 'truncate' }, safeText(selected.displayName ?? selected.name), selected.enabled ? ' · Enabled' : ' · Disabled'),
        Text({ wrap: 'truncate' }, safeText(selected.description ?? '')),
        ...(selected.shortDescription ? [Text({ wrap: 'truncate' }, safeText(selected.shortDescription))] : []),
        Text({ dim: true, wrap: 'truncate' }, safeText(selected.path)),
        ...(selected.dependencies?.length ? [Text({ wrap: 'truncate' }, safeText(selected.dependencies.map(tool =>
          `${tool.type}: ${tool.value}${tool.description ? ` – ${tool.description}` : ''}`).join(' · ')))] : []))] : []),
      ...(status() || warning ? [Text({ color: warning && !status() ? 'yellow' : accent, wrap: 'truncate' }, status() || safeText(warning!))] : []),
      instructions('↑ ↓  Move    Enter  Edit / Toggle    Ctrl+S  Save    Ctrl+R  Refresh    Esc  Back'));
  });
  await runTui(() => Editor({ key: 'editor' }), screen, mounted => { app = mounted; });
}
