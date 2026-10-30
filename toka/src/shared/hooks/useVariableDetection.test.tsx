import { act, renderHook } from '@testing-library/react';
import type { SuperDocRef } from '@superdoc-dev/react';
import { describe, expect, it } from 'vitest';
import { useVariableDetection } from './useVariableDetection';

function detect(html: string) {
  const ref = { current: { getInstance: () => ({ getHTML: () => [html] }) } } as unknown as React.RefObject<SuperDocRef | null>;
  const hook = renderHook(() => useVariableDetection(ref));
  act(() => hook.result.current.triggerScan());
  return hook;
}

describe('useVariableDetection', () => {
  it('identifies dates and qualified English fields split between Word formatting spans', () => {
    const hook = detect('<p>{{current_date_<strong>plus_30</strong>_days}}</p><p>{{buyer.<span>nationality</span>}}</p><p>{{custom_clause}}</p>');
    expect(hook.result.current.allFoundKeys).toEqual(['current_date_plus_30_days', 'buyer.nationality', 'custom_clause']);
    expect(hook.result.current.identifiedVars.map((variable) => variable.key)).toEqual(['current_date_plus_30_days', 'buyer.nationality']);
    expect(hook.result.current.unidentifiedKeys).toEqual(['custom_clause']);
    hook.unmount();
  });

  it('keeps loop item fields local and never joins tags across paragraph boundaries', () => {
    const hook = detect('<p>{{#buyers}}</p><p>{{name}} / {{nationality}} / {{current_date_plus_30_days}}</p><p>{{/buyers}}</p><p>{</p><p>{current_date}}</p><p>{{current_date_plus_2_business_days_iso}}</p>');
    expect(hook.result.current.allFoundKeys).toEqual(['buyers', 'current_date_plus_30_days', 'current_date_plus_2_business_days_iso']);
    expect(hook.result.current.unidentifiedKeys).toEqual([]);
    hook.unmount();
  });
});
