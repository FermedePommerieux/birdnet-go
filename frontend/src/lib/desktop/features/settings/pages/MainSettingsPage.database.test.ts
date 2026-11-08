import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { settingsStore, type OutputSettings, type SettingsFormData } from '$lib/stores/settings';

vi.mock('$lib/utils/api', async importOriginal => ({
  ...(await importOriginal<typeof import('$lib/utils/api')>()),
  api: { get: vi.fn().mockResolvedValue({}) },
}));

import MainSettingsPage from './MainSettingsPage.svelte';

const output: OutputSettings = {
  sqlite: { enabled: false, path: 'birdnet.db' },
  mysql: {
    enabled: false,
    host: 'mysql.example',
    port: '3306',
    username: 'mysqluser',
    password: 'mysqlpass',
    database: 'birdnet',
  },
  postgresql: {
    enabled: true,
    host: 'pg.example',
    port: '5432',
    username: 'pguser',
    password: 'pgpass',
    database: 'birds',
    sslMode: 'verify-full',
    sslRootCert: '/certs/ca.pem',
    sslCert: '',
    sslKey: '',
  },
};

function setOutput(value: OutputSettings) {
  const formData = {
    main: { name: 'TestNode' },
    output: structuredClone(value),
  } as SettingsFormData;
  settingsStore.update(state => ({
    ...state,
    formData,
    originalData: structuredClone(formData),
    isLoading: false,
    isSaving: false,
  }));
}

describe('MainSettingsPage database settings', () => {
  beforeEach(() => {
    Element.prototype.scrollIntoView = vi.fn();
    setOutput(output);
  });

  afterEach(() => cleanup());

  async function openDatabaseTab() {
    render(MainSettingsPage);
    await fireEvent.click(screen.getByRole('tab', { name: /database/i }));
  }

  it('shows PostgreSQL connection and TLS controls when it is selected', async () => {
    await openDatabaseTab();

    expect(
      screen.getByRole('combobox', { name: 'settings.main.sections.database.type.label' })
    ).toHaveTextContent('PostgreSQL');
    expect(document.getElementById('postgresql-host')).toHaveValue('pg.example');
    expect(document.getElementById('postgresql-ssl-root-cert')).toHaveValue('/certs/ca.pem');

    await fireEvent.change(document.getElementById('postgresql-host')!, {
      target: { value: 'new-pg.example' },
    });
    expect(get(settingsStore).formData.output?.postgresql?.host).toBe('new-pg.example');
    expect(get(settingsStore).formData.output?.mysql.host).toBe('mysql.example');

    await fireEvent.click(
      screen.getByRole('combobox', {
        name: 'settings.main.sections.database.postgresql.sslMode.label',
      })
    );
    await fireEvent.click(screen.getByRole('option', { name: 'require' }));
    expect(get(settingsStore).formData.output?.postgresql?.sslMode).toBe('require');
  });

  it('switches engines without discarding saved connection values', async () => {
    await openDatabaseTab();
    await fireEvent.click(
      screen.getByRole('combobox', { name: 'settings.main.sections.database.type.label' })
    );
    await fireEvent.click(screen.getByRole('option', { name: /MySQL/i }));

    await waitFor(() => {
      const changed = get(settingsStore).formData.output!;
      expect(changed.mysql.enabled).toBe(true);
      expect(changed.postgresql?.enabled).toBe(false);
      expect(changed.sqlite.enabled).toBe(false);
      expect(changed.postgresql?.host).toBe('pg.example');
      expect(changed.postgresql?.password).toBe('pgpass');
    });
  });

  it('allows choosing PostgreSQL from SQLite', async () => {
    setOutput({
      ...output,
      sqlite: { ...output.sqlite, enabled: true },
      postgresql: { ...output.postgresql!, enabled: false },
    });
    await openDatabaseTab();
    await fireEvent.click(
      screen.getByRole('combobox', { name: 'settings.main.sections.database.type.label' })
    );
    await fireEvent.click(screen.getByRole('option', { name: /PostgreSQL/i }));

    await waitFor(() => {
      const changed = get(settingsStore).formData.output!;
      expect(changed.postgresql?.enabled).toBe(true);
      expect(changed.sqlite.enabled).toBe(false);
      expect(changed.mysql.enabled).toBe(false);
      expect(changed.postgresql?.host).toBe('pg.example');
    });
  });
});
