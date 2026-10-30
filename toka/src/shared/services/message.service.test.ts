import { describe, expect, it, vi } from 'vitest';
import { getOtherChannelParticipant, normalizeMessageChannel, type Channel } from './message.service';

vi.mock('./api', () => ({ default: {} }));

const rawChannel = {
 id: 'conversation', name: 'Conversa', type: 'direct',
 participants: ['broker-account', 'client-account'],
 participant_profiles: [
  { id: 'broker-account', name: 'Juliana Cabral', avatar_url: '/juliana.png' },
  { id: 'client-account', name: 'Lucas Mendes Barros', avatar_url: '/lucas.png' },
 ],
} as unknown as Channel;

describe('chat participant identities', () => {
 it('uses the client account profile for the broker list and header', () => {
  expect(getOtherChannelParticipant(rawChannel, 'broker-account')).toMatchObject({
   id: 'client-account', name: 'Lucas Mendes Barros', avatar_url: '/lucas.png',
  });
 });
 it('handles enriched participant objects and repeated normalization', () => {
  const normalized = normalizeMessageChannel(rawChannel);
  expect(normalizeMessageChannel(normalized)).toEqual(normalized);
  expect(getOtherChannelParticipant(normalized, 'client-account')).toMatchObject({
   id: 'broker-account', name: 'Juliana Cabral', avatar_url: '/juliana.png',
  });
 });
 it('keeps participant identity when a profile has no photo or is unavailable', () => {
  const channel = { ...rawChannel, participant_profiles: [{ id: 'client-account', name: 'Lucas Mendes Barros' }] };
  expect(getOtherChannelParticipant(channel, 'broker-account')).toMatchObject({ id: 'client-account', name: 'Lucas Mendes Barros' });
  expect(getOtherChannelParticipant(channel, 'client-account')?.id).toBe('broker-account');
 });
});
