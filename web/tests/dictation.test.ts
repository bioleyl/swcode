import { describe, expect, it } from 'vitest';

import { spokenText, textWithSpoken } from '../src/lib/dictation';

describe('what was said so far', () => {
  it('is nothing before the first word', () => {
    expect(spokenText([])).toBe('');
    expect(spokenText([''])).toBe('');
  });

  it('joins the results one space apart, whether the browser starts them with a space or not', () => {
    expect(spokenText(['fix the'])).toBe('fix the');
    expect(spokenText(['fix the', ' login bug'])).toBe('fix the login bug');
    expect(spokenText(['fix the', 'login bug'])).toBe('fix the login bug');
  });

  it('leaves out a result with no words', () => {
    expect(spokenText(['fix the', ' ', 'login bug'])).toBe('fix the login bug');
  });
});

describe('the words said, in the field', () => {
  it('fill an empty field, the caret after them', () => {
    expect(textWithSpoken({ after: '', before: '' }, 'fix the bug')).toEqual({ caret: 11, text: 'fix the bug' });
  });

  it('leave the field as it was while nothing is heard', () => {
    expect(textWithSpoken({ after: ' later', before: 'Then' }, '')).toEqual({ caret: 4, text: 'Then later' });
  });

  it('come a space after the words before the caret', () => {
    expect(textWithSpoken({ after: '', before: 'Please' }, 'fix it')).toEqual({
      caret: 13,
      text: 'Please fix it',
    });
    expect(textWithSpoken({ after: '', before: 'Please ' }, 'fix it')).toEqual({
      caret: 13,
      text: 'Please fix it',
    });
  });

  it('keep a space before the words after the caret, but not before punctuation', () => {
    expect(textWithSpoken({ after: 'today', before: '' }, 'fix it')).toEqual({ caret: 6, text: 'fix it today' });
    expect(textWithSpoken({ after: ' today', before: '' }, 'fix it')).toEqual({ caret: 6, text: 'fix it today' });
    expect(textWithSpoken({ after: ', today', before: 'Please' }, 'fix it')).toEqual({
      caret: 13,
      text: 'Please fix it, today',
    });
  });
});
