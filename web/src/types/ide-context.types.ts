/** One problem the host's language server reports, with the place it was found at. */
export interface IdeDiagnostic {
  /** 1-based, as the editor shows it. */
  line: number;
  /** 1-based, as the editor shows it. */
  column: number;
  severity: string;
  message: string;
  /** The tool that found it (an editor's language server, a linter); empty when it names none. */
  source: string;
  /** The rule it was found under (`no-unused-vars`); empty when it names none. */
  code: string;
}

/** What the host editor is looking at: a file, the part of it selected, and the problems found in it. */
export interface IdeContext {
  path: string;
  /** The editor's language, so the code block says what it is (`typescript`). */
  language: string;
  selection: IdeSelection;
  diagnostics: IdeDiagnostic[];
}

/** The part of the file selected: a caret is a range with both ends on the same place. */
export interface IdeSelection {
  startLine: number;
  startColumn: number;
  endLine: number;
  endColumn: number;
  text: string;
}
