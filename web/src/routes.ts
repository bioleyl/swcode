import { defineRoutes } from '@jadis/core';

import { HomePage } from './pages/home-page';
import { NewChatPage } from './pages/new-chat-page';
import { ProjectPage } from './pages/project-page';
import { SessionPage } from './pages/session-page';
import { SessionsPage } from './pages/sessions-page';
import { SettingsPage } from './pages/settings-page';

/** The pages, most specific path first: the router takes the first that matches, so the catch-all comes last. */
// biome-ignore assist/source/useSortedKeys: the order of the routes is the order they are matched in
export const ROUTES = defineRoutes({
  session: { page: SessionPage, path: '/sessions/:id' },
  sessions: { page: SessionsPage, path: '/sessions' },
  projectTab: { page: ProjectPage, path: '/projects/:id/:tab' },
  project: { page: ProjectPage, path: '/projects/:id' },
  // The same page as home: the form for a new session.
  newInProject: { page: HomePage, path: '/new/:project' },
  newSession: { page: HomePage, path: '/new' },
  newChat: { page: NewChatPage, path: '/chat' },
  settingsTopic: { page: SettingsPage, path: '/settings/:topic' },
  settings: { page: SettingsPage, path: '/settings' },
  home: { page: HomePage, path: '/' },
  notFound: { page: HomePage, path: '/:rest' },
});

export type RouteName = Extract<keyof typeof ROUTES, string>;
