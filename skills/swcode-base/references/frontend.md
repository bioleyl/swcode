# Frontend: services and atomic components

Details for rules 14–16. The examples use Jadis; the split is the same in any framework. They declare types inline to stay short; in real code they live in `types/`.

## Why

The UI changes often and for its own reasons: a redesign, a new layout, a component swapped for another. Business rules change for other reasons, at another pace. Keeping them apart means a UI change can't break a rule, and the rules can be tested without rendering anything.

## Layers

The folder names below are the usual ones; follow the project's own. What matters is the split: rules out of components, and layers that use only their own or lower ones.

| Folder | Holds | Built from, may call | Never |
|---|---|---|---|
| `services/` | Business rules, app state, use cases (sign in, delete a workflow). `api.ts` is the one gateway to the server; a site with a single call keeps it in one module. | The gateway, `lib/`, other services | Components, the DOM, `fetch` outside `api.ts` |
| `lib/` | Pure calculations: rules that need no state, mappings, labels, formatting | Other `lib/` files, translation | I/O, state |
| `atoms/` | One element with its look: a chip, a brand mark, a step icon | The design system's components | Services, except presentational ones |
| `molecules/` | A few atoms with one purpose: a user row, a workflow card | Other molecules, atoms | Services, except presentational ones |
| `organisms/` | A section of a page with its own behaviour: a form, a list, a dialog | Other organisms, molecules, atoms, services | Business rules |
| `templates/` | Page layout with slots, no data | Lower layers | Services, data |
| `pages/` | One per route: wires services to organisms | All of the above, services | Business rules |

A component may contain others from its own layer (a dialog organism around a form organism); it never uses a higher layer.

**Presentational services** (translation, theme, date and number formatting) can be used anywhere: they change how things are shown, never what the app decides.

## Data down, events up

An organism shows what it's given and reports what the user did. It doesn't know what happens next.

```tsx
// organisms/sw-login-form.tsx
export class SwLoginForm extends JlElement {
  static readonly selector = 'sw-login-form';

  readonly events = this.useEvents<{ submit: Login }>();

  setBusy(busy: boolean): void {
    // …
  }

  showError(message: string): void {
    // …
  }
}
```

The page connects it to the service:

```tsx
// pages/login-page.tsx
<SwLoginForm onSubmit={(login) => void this.submit(login)} />

private async submit(login: Login): Promise<void> {
  const form = this._refs.form;
  form.setBusy(true);
  try {
    await signIn(login.username, login.password);
  } catch (error) {
    form.showError(messageOf(error));
  } finally {
    form.setBusy(false);
  }
}
```

The service holds the use case, and talks to the server only through the gateway:

```ts
// services/session.ts
export async function signIn(username: string, password: string): Promise<User> {
  const user = await api.auth.login({ password, username });
  state.user = user;
  sessionBus.emit('signedIn', user);
  return user;
}
```

The form can be redesigned, or replaced by a passkey button, without touching `signIn`.

## A plain call or a decision?

Organisms and pages may call services, including the gateway for a plain read or write: `api.workflows.list()` needs no wrapper, and a service that only passes the call through would be speculation (rule 13). As soon as the call needs a decision, the decision moves into a service.

```tsx
// Bad: organisms/sw-workflow-actions.tsx
const deletable = currentUser()?.role === 'admin' && workflow.runCount === 0;
```

Who may delete a workflow is a business rule. Inside a component, it can't be tested without rendering, and the next component that shows a delete button will copy it, or forget it.

```ts
// Good: services/workflows.ts
export const canDeleteWorkflow = (workflow: Pick<Workflow, 'runCount'>, user: Pick<User, 'role'>): boolean =>
  user.role === 'admin' && workflow.runCount === 0;
```

Signs that a decision is hiding in a component: a condition on a role or a status, a calculation on amounts or dates, a choice of what to do after a particular error, a rule copied in two components.

## Services and the UI

A service never imports a component or reaches into the DOM. When a use case needs app-wide UI (a toast, a confirm dialog, navigation), it calls a function for it, so a test can replace that function and check the rule alone.

## Testing the split

- **`lib/`:** plain unit tests, no DOM, no fakes.
- **Services:** replace the gateway (a module mock, or a fake passed in) and assert on what the service returns, the state it keeps, and the calls it makes.
- **Components:** render tests (jsdom) check that the data shows and the events fire. They need no business rule to set up; if one does, the rule is in the wrong place.
