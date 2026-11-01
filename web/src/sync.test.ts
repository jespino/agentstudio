import { afterEach, expect, it, vi } from 'vitest';
import type { Backend } from './backend';
import { configureStudio } from './config';
import { createSourceSaver, onWorkspace } from './sync';

function fakeBackend() {
  const listeners = new Map<string, (data: unknown) => void>();
  const call = vi.fn().mockResolvedValue(undefined);
  const backend: Backend = {
    isWails: false,
    call: call as Backend['call'],
    on: <T>(name: string, fn: (data: T) => void) => {
      listeners.set(name, fn as (data: unknown) => void);
      return () => listeners.delete(name);
    },
    broadcast: async () => {},
  };
  configureStudio({ backend, id: 'test' });
  return { call, emit: (name: string, data: unknown) => listeners.get(name)?.(data), listeners };
}

afterEach(() => vi.useRealTimers());

it('saves the source once typing stops, and not after a replace', async () => {
  vi.useFakeTimers();
  const { call } = fakeBackend();
  const saver = createSourceSaver(400);
  saver.schedule('a');
  saver.schedule('ab');
  await vi.advanceTimersByTimeAsync(399);
  expect(call).not.toHaveBeenCalled();
  await vi.advanceTimersByTimeAsync(1);
  expect(call).toHaveBeenCalledExactlyOnceWith('SaveSource', 'ab');
  saver.schedule('abc');
  saver.cancel();
  await vi.advanceTimersByTimeAsync(1000);
  expect(call).toHaveBeenCalledTimes(1);
  // flush saves the pending text at once, and only once
  saver.schedule('abcd');
  saver.flush();
  expect(call).toHaveBeenLastCalledWith('SaveSource', 'abcd');
  await vi.advanceTimersByTimeAsync(1000);
  expect(call).toHaveBeenCalledTimes(2);
});

it('passes on the project and only outside edits of its file', () => {
  const { emit, listeners } = fakeBackend();
  const projects: unknown[] = [];
  const sources: string[] = [];
  const off = onWorkspace({
    onProject: (p) => projects.push(p),
    onSource: (t) => sources.push(t),
    current: () => ({ path: '/p/a.demo', source: 'mine' }),
  });
  emit('project:changed', null);
  emit('project:source', { path: '/p/a.demo', source: 'mine' }); // our own save
  emit('project:source', { path: '/p/other.demo', source: 'x' }); // another project
  emit('project:source', { path: '/p/a.demo', source: 'agent' });
  expect(projects).toEqual([null]);
  expect(sources).toEqual(['agent']);
  off();
  expect(listeners.size).toBe(0);
});
