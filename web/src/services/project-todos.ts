import { daemon } from './daemon';

// The todos live on the project, which the daemon sends back changed with the projects: no read of its own.

/** Adds a todo to the project, at the end of its list. */
export const addProjectTodo = (projectId: string, text: string): void =>
  daemon.send({ projectId, text, type: 'addProjectTodo' });

/** Marks one of the project's todos done or not. */
export const setProjectTodoDone = (projectId: string, todoId: string, done: boolean): void =>
  daemon.send({ done, projectId, todoId, type: 'setProjectTodoDone' });

/** Changes what one of the project's todos says. */
export const setProjectTodoText = (projectId: string, todoId: string, text: string): void =>
  daemon.send({ projectId, text, todoId, type: 'setProjectTodoText' });

/** Deletes one of the project's todos. */
export const removeProjectTodo = (projectId: string, todoId: string): void =>
  daemon.send({ projectId, todoId, type: 'removeProjectTodo' });
