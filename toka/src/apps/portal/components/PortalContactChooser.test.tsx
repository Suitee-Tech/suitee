import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, useLocation } from 'react-router';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { PortalContactChooser } from './PortalContactChooser';
const mocks = vi.hoisted(() => ({ getOverview: vi.fn(), getDeals: vi.fn(), openDirectChannel: vi.fn(), openDealChannel: vi.fn(), onOpened: vi.fn(), error: vi.fn() }));
vi.mock('../services/portal.service', () => ({ portalService: mocks }));
vi.mock('sonner', () => ({ toast: { error: mocks.error } }));
function Location() { const location = useLocation(); return <output>{location.pathname}{location.search}</output>; }
function page() { return <MemoryRouter initialEntries={['/portal/mensagens?contact=1']}><PortalContactChooser onOpened={mocks.onOpened} /><Location /></MemoryRouter>; }
beforeEach(() => {
 vi.clearAllMocks(); mocks.getOverview.mockResolvedValue([{ id:'my-lead',name:'Atendimento',broker_name:'Juliana' }]);
 mocks.getDeals.mockResolvedValue([{ id:'my-deal',display_id:'NEG-42' }]);
 mocks.openDirectChannel.mockResolvedValue({channel_id:'existing-channel'});
 mocks.openDealChannel.mockResolvedValue({id:'deal-channel'});
});
describe('portal contact entry', () => {
 it('opens the persisted broker conversation from the chosen lead', async () => {
  render(page()); fireEvent.click(await screen.findByRole('button',{name:/Juliana/}));
  await waitFor(() => expect(screen.getByText('/portal/mensagens?channel=existing-channel')).toBeInTheDocument());
  expect(mocks.openDirectChannel).toHaveBeenCalledWith('my-lead');
  expect(mocks.onOpened).toHaveBeenCalledOnce();
 });
 it('can open a deal conversation for a client without a lead', async () => {
  mocks.getOverview.mockResolvedValue([]); render(page());
  fireEvent.click(await screen.findByRole('button',{name:/NEG-42/}));
  await waitFor(() => expect(screen.getByText('/portal/mensagens?channel=deal-channel')).toBeInTheDocument());
  expect(mocks.openDealChannel).toHaveBeenCalledWith('my-deal');
 });
 it('keeps contact selection visible when no channel was returned', async () => {
  mocks.openDirectChannel.mockResolvedValue({}); render(page());
  fireEvent.click(await screen.findByRole('button',{name:/Juliana/}));
  await waitFor(() => expect(mocks.error).toHaveBeenCalled());
  expect(screen.getByText('/portal/mensagens?contact=1')).toBeInTheDocument();
  expect(mocks.onOpened).not.toHaveBeenCalled();
 });
 it('shows available contacts and retry when one source fails', async () => {
  mocks.getOverview.mockRejectedValue(new Error('offline')); render(page());
  expect(await screen.findByRole('button',{name:/NEG-42/})).toBeInTheDocument();
  expect(screen.getByRole('alert')).toBeInTheDocument();
  mocks.getOverview.mockResolvedValue([{id:'my-lead',broker_name:'Juliana'}]);
  fireEvent.click(screen.getByRole('button',{name:'Tentar novamente'}));
  expect(await screen.findByRole('button',{name:/Juliana/})).toBeInTheDocument();
 });
});
