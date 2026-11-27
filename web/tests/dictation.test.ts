import { describe, expect, it, vi } from 'vitest';

import { dictationOnceAllowed, spokenText, textWithSpoken } from '../src/lib/dictation';

const REFUSED = 'The microphone is not allowed.';

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

describe('a dictation that asks for the microphone first', () => {
  function asked() {
    const answer = Promise.withResolvers<boolean>();
    const recognizer = { abort: vi.fn(), start: vi.fn(), stop: vi.fn() };
    const listener = { ended: vi.fn(), failed: vi.fn() };
    const dictation = dictationOnceAllowed(answer.promise, recognizer, listener, REFUSED);
    const answered = async (isAllowed: boolean): Promise<void> => {
      answer.resolve(isAllowed);
      await answer.promise;
    };
    return { answered, dictation, listener, recognizer };
  }

  it('listens only once the microphone is allowed, and then stops through the recognizer', async () => {
    const { answered, dictation, listener, recognizer } = asked();
    expect(recognizer.start).not.toHaveBeenCalled();

    await answered(true);
    expect(recognizer.start).toHaveBeenCalledOnce();

    dictation.stop();
    expect(recognizer.stop).toHaveBeenCalledOnce();
    // The recognizer says its own end.
    expect(listener.ended).not.toHaveBeenCalled();
  });

  it('ends saying why when the microphone is refused, without listening', async () => {
    const { answered, listener, recognizer } = asked();

    await answered(false);

    expect(recognizer.start).not.toHaveBeenCalled();
    expect(listener.failed).toHaveBeenCalledWith(REFUSED);
    expect(listener.ended).toHaveBeenCalledOnce();
  });

  it('never listens when it is stopped while the question shows', async () => {
    const { answered, dictation, listener, recognizer } = asked();

    dictation.stop();
    expect(listener.ended).toHaveBeenCalledOnce();

    await answered(true);
    dictation.abort();
    expect(recognizer.start).not.toHaveBeenCalled();
    expect(recognizer.abort).not.toHaveBeenCalled();
    expect(listener.ended).toHaveBeenCalledOnce();
  });
});
