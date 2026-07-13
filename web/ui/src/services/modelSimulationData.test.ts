import { describe, expect, it } from 'vitest';
import { buildModelSimulationPage } from './modelSimulationData';

describe('buildModelSimulationPage', () => {
  it('keeps API page records first and fills only the remaining page slots', () => {
    const apiRow = { __model_id: 'API-PAGE-2', 模型名: '真实第二页模型' };
    const rows = buildModelSimulationPage([apiRow], 28, 2, 8);
    expect(rows).toHaveLength(8);
    expect(rows[0]).toEqual(apiRow);
    expect(rows[1].__model_id).toBe('SIM-MODEL-010');
    expect(rows[1].__data_mode).toBe('simulated');
  });
});
