import { Alert, Button, Empty, Input, InputNumber, Segmented, Select, Space, Switch, Tag, Tooltip } from 'antd';
import { DeleteOutlined, FilterOutlined, PlusOutlined, SettingOutlined, SlidersOutlined } from '@ant-design/icons';
import type { ReactNode } from 'react';
import { useEffect, useMemo, useRef, useState } from 'react';

export interface StrategyFactorParamDefinition {
  name: string;
  label: string;
  type: string;
  default?: unknown;
}

export interface StrategyFactorDefinition {
  id: string;
  name: string;
  kind: string;
  description?: string;
  params?: StrategyFactorParamDefinition[];
}

interface StrategyRule {
  id?: string;
  factor: string;
  weight?: number;
  params?: Record<string, unknown>;
}

interface StrategyEditorConfig {
  [key: string]: unknown;
  filters: StrategyRule[];
  scores: StrategyRule[];
  pass: {
    min_score?: number;
    top_n?: number;
    [key: string]: unknown;
  };
  execution: {
    min_open_gap?: number;
    max_open_gap?: number;
    [key: string]: unknown;
  };
}

interface StrategyFactorEditorProps {
  value?: string;
  onChange?: (value: string) => void;
  factors: StrategyFactorDefinition[];
  disabled?: boolean;
}

type RuleSection = 'filters' | 'scores';

function makeRuleID(factorID: string) {
  return `${factorID}-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 7)}`;
}

function formatJSON(value: unknown) {
  return JSON.stringify(value, null, 2);
}

function normalizeRules(value: unknown): StrategyRule[] {
  if (!Array.isArray(value)) return [];
  return value
    .filter((item): item is Record<string, unknown> => Boolean(item && typeof item === 'object' && !Array.isArray(item)))
    .map((item) => ({
      ...item,
      factor: typeof item.factor === 'string' ? item.factor : '',
      params: item.params && typeof item.params === 'object' && !Array.isArray(item.params)
        ? item.params as Record<string, unknown>
        : {}
    })) as StrategyRule[];
}

function normalizeConfig(value?: string): StrategyEditorConfig {
  let parsed: Record<string, unknown> = {};
  try {
    const next = JSON.parse(value || '{}') as unknown;
    if (next && typeof next === 'object' && !Array.isArray(next)) {
      parsed = next as Record<string, unknown>;
    }
  } catch {
    // The JSON editor keeps invalid text visible; visual mode starts from a safe shape.
  }
  return {
    ...parsed,
    filters: normalizeRules(parsed.filters),
    scores: normalizeRules(parsed.scores),
    pass: parsed.pass && typeof parsed.pass === 'object' && !Array.isArray(parsed.pass)
      ? parsed.pass as StrategyEditorConfig['pass']
      : {},
    execution: parsed.execution && typeof parsed.execution === 'object' && !Array.isArray(parsed.execution)
      ? parsed.execution as StrategyEditorConfig['execution']
      : {}
  };
}

function defaultParams(definition?: StrategyFactorDefinition) {
  return Object.fromEntries((definition?.params || []).map((param) => [param.name, param.default ?? '']));
}

function ruleLabel(rule: StrategyRule, factors: StrategyFactorDefinition[]) {
  return factors.find((factor) => factor.id === rule.factor)?.name || rule.factor || '未选择因子';
}

function numberValue(value: unknown, fallback = 0) {
  const next = Number(value);
  return Number.isFinite(next) ? next : fallback;
}

export function StrategyFactorEditor({ value, onChange, factors, disabled = false }: StrategyFactorEditorProps) {
  const initialConfig = normalizeConfig(value);
  const [config, setConfig] = useState<StrategyEditorConfig>(initialConfig);
  const [jsonText, setJsonText] = useState(formatJSON(initialConfig));
  const [jsonError, setJsonError] = useState('');
  const [mode, setMode] = useState<'visual' | 'json'>('visual');
  const [search, setSearch] = useState('');
  const lastEmittedValue = useRef(value || formatJSON(initialConfig));

  useEffect(() => {
    const externalValue = value || '{}';
    if (externalValue === lastEmittedValue.current) return;
    const next = normalizeConfig(externalValue);
    setConfig(next);
    setJsonText(formatJSON(next));
    setJsonError('');
    lastEmittedValue.current = externalValue;
  }, [value]);

  const filteredFactors = useMemo(() => {
    const keyword = search.trim().toLowerCase();
    if (!keyword) return factors;
    return factors.filter((factor) => `${factor.name} ${factor.id} ${factor.description || ''}`.toLowerCase().includes(keyword));
  }, [factors, search]);

  const filterDefinitions = filteredFactors.filter((factor) => factor.kind === 'filter');
  const scoreDefinitions = filteredFactors.filter((factor) => factor.kind === 'score');

  function emit(next: StrategyEditorConfig) {
    const serialized = formatJSON(next);
    setConfig(next);
    setJsonText(serialized);
    setJsonError('');
    lastEmittedValue.current = serialized;
    onChange?.(serialized);
  }

  function updateRule(section: RuleSection, index: number, patch: Partial<StrategyRule>) {
    const rules = [...config[section]];
    rules[index] = { ...rules[index], ...patch };
    emit({ ...config, [section]: rules });
  }

  function removeRule(section: RuleSection, index: number) {
    emit({ ...config, [section]: config[section].filter((_rule, ruleIndex) => ruleIndex !== index) });
  }

  function addRule(section: RuleSection, definition: StrategyFactorDefinition) {
    const rule: StrategyRule = {
      id: makeRuleID(definition.id),
      factor: definition.id,
      params: defaultParams(definition)
    };
    if (section === 'scores') rule.weight = 10;
    emit({ ...config, [section]: [...config[section], rule] });
  }

  function changeRuleFactor(section: RuleSection, index: number, factorID: string) {
    const definition = factors.find((factor) => factor.id === factorID);
    updateRule(section, index, { factor: factorID, params: defaultParams(definition) });
  }

  function updateParam(section: RuleSection, index: number, name: string, nextValue: unknown) {
    const rule = config[section][index];
    updateRule(section, index, { params: { ...(rule.params || {}), [name]: nextValue } });
  }

  function updateConfigField(key: string, nextValue: unknown) {
    emit({ ...config, [key]: nextValue });
  }

  function updatePassField(key: string, nextValue: unknown) {
    emit({ ...config, pass: { ...config.pass, [key]: nextValue } });
  }

  function updateExecutionField(key: string, nextValue: number | null) {
    const execution = { ...config.execution };
    if (nextValue === null) delete execution[key];
    else execution[key] = nextValue;
    emit({ ...config, execution });
  }

  function applyJSON() {
    try {
      const parsed = JSON.parse(jsonText) as unknown;
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
        throw new Error('配置必须是 JSON 对象');
      }
      emit(normalizeConfig(jsonText));
      setMode('visual');
    } catch (error) {
      setJsonError(error instanceof Error ? error.message : 'JSON 格式错误');
    }
  }

  function switchMode(nextMode: 'visual' | 'json') {
    if (nextMode === 'visual' && mode === 'json') {
      try {
        const parsed = JSON.parse(jsonText) as unknown;
        if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) throw new Error('配置必须是 JSON 对象');
        emit(normalizeConfig(jsonText));
      } catch (error) {
        setJsonError(error instanceof Error ? error.message : 'JSON 格式错误');
        return;
      }
    }
    setMode(nextMode);
  }

  function renderParam(section: RuleSection, index: number, rule: StrategyRule, param: StrategyFactorParamDefinition) {
    const current = rule.params?.[param.name] ?? param.default;
    if (param.type === 'boolean') {
      return <Switch size="small" checked={Boolean(current)} onChange={(checked) => updateParam(section, index, param.name, checked)} disabled={disabled} />;
    }
    if (param.type === 'number') {
      return <InputNumber size="small" value={numberValue(current)} onChange={(next) => updateParam(section, index, param.name, next ?? 0)} disabled={disabled} controls />;
    }
    return <Input size="small" value={String(current ?? '')} onChange={(event) => updateParam(section, index, param.name, event.target.value)} disabled={disabled} />;
  }

  function renderRule(section: RuleSection, rule: StrategyRule, index: number) {
    const definition = factors.find((factor) => factor.id === rule.factor);
    const options = factors.filter((factor) => factor.kind === section.slice(0, -1)).map((factor) => ({ label: factor.name, value: factor.id }));
    return (
      <div className="strategy-factor-rule" key={rule.id || `${rule.factor}-${index}`}>
        <div className="strategy-factor-rule-head">
          <div className="strategy-factor-rule-selector">
            <Tag color={section === 'filters' ? 'gold' : 'blue'}>{section === 'filters' ? '过滤' : '评分'}</Tag>
            <Select
              size="small"
              value={definition ? rule.factor : undefined}
              placeholder={rule.factor || '选择因子'}
              options={options}
              onChange={(next) => changeRuleFactor(section, index, next)}
              disabled={disabled}
              showSearch
              optionFilterProp="label"
            />
            {definition ? <Tooltip title={definition.description}><span className="strategy-factor-rule-id">{definition.id}</span></Tooltip> : <span className="strategy-factor-rule-id is-warning">因子定义不存在</span>}
          </div>
          <Space size={4}>
            {section === 'scores' ? <label className="strategy-factor-weight"><span>权重</span><InputNumber size="small" min={0} value={numberValue(rule.weight, 10)} onChange={(next) => updateRule(section, index, { weight: next ?? 0 })} disabled={disabled} /></label> : null}
            <Button aria-label={`删除${ruleLabel(rule, factors)}`} title="删除因子" type="text" size="small" danger icon={<DeleteOutlined />} onClick={() => removeRule(section, index)} disabled={disabled} />
          </Space>
        </div>
        {definition?.params?.length ? (
          <div className="strategy-factor-param-grid">
            {definition.params.map((param) => (
              <label key={param.name} className={`strategy-factor-param strategy-factor-param-${param.type}`}>
                <span>{param.label || param.name}</span>
                {renderParam(section, index, rule, param)}
              </label>
            ))}
          </div>
        ) : <div className="strategy-factor-no-params">这个因子不需要额外参数。</div>}
      </div>
    );
  }

  function renderRuleSection(section: RuleSection, title: string, hint: string, icon: ReactNode) {
    const definitions = section === 'filters' ? filterDefinitions : scoreDefinitions;
    return (
      <section className={`strategy-factor-section strategy-factor-section-${section}`}>
        <div className="strategy-factor-section-head">
          <div><span className="strategy-factor-section-icon">{icon}</span><strong>{title}</strong><small>{hint}</small></div>
          <span className="strategy-factor-count">{config[section].length} 项</span>
        </div>
        <div className="strategy-factor-rule-list">
          {config[section].map((rule, index) => renderRule(section, rule, index))}
          {!config[section].length ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={section === 'filters' ? '还没有硬过滤条件' : '还没有评分因子'} /> : null}
        </div>
        <div className="strategy-factor-add-row">
          <Select
            size="small"
            className="strategy-factor-add-select"
            placeholder={`添加${title}`}
            options={definitions.map((factor) => ({ label: `${factor.name} · ${factor.id}`, value: factor.id }))}
            onChange={(factorID) => {
              const definition = factors.find((factor) => factor.id === factorID);
              if (definition) addRule(section, definition);
            }}
            value={undefined}
            disabled={disabled}
            showSearch
            optionFilterProp="label"
          />
          <span>{definitions.length} 个可用因子</span>
        </div>
      </section>
    );
  }

  return (
    <div className="strategy-factor-editor">
      <div className="strategy-factor-editor-toolbar">
        <div>
          <strong>因子编排台</strong>
          <span>拖动感不需要, 选择即可把规则写入策略 JSON</span>
        </div>
        <Segmented
          size="small"
          value={mode}
          onChange={(next) => switchMode(next as 'visual' | 'json')}
          options={[{ label: '可视化', value: 'visual' }, { label: 'JSON 高级', value: 'json' }]}
        />
      </div>

      {mode === 'json' ? (
        <div className="strategy-factor-json-mode">
          <Input.TextArea value={jsonText} onChange={(event) => { setJsonText(event.target.value); setJsonError(''); lastEmittedValue.current = event.target.value; onChange?.(event.target.value); }} disabled={disabled} autoSize={{ minRows: 20, maxRows: 28 }} spellCheck={false} />
          <div className="strategy-factor-json-actions">
            <span>{jsonError || '高级模式允许保留尚未被可视化编辑器识别的配置字段。'}</span>
            <Button size="small" type="primary" onClick={applyJSON} disabled={disabled}>应用到可视化</Button>
          </div>
          {jsonError ? <Alert type="error" showIcon message={jsonError} /> : null}
        </div>
      ) : (
        <div className="strategy-factor-visual-mode">
          <div className="strategy-factor-editor-layout">
            <div className="strategy-factor-canvas">
              {renderRuleSection('filters', '硬过滤', '先过滤掉不符合条件的标的', <FilterOutlined />)}
              {renderRuleSection('scores', '评分因子', '命中后按权重累加，决定最终排序', <SlidersOutlined />)}

              <section className="strategy-factor-runtime">
                <div className="strategy-factor-section-head">
                  <div><span className="strategy-factor-section-icon"><SettingOutlined /></span><strong>通过与运行</strong><small>控制门槛、历史数据和执行规模</small></div>
                </div>
                <div className="strategy-factor-runtime-grid">
                  <label><span>最低分</span><InputNumber size="small" min={0} value={numberValue(config.pass.min_score, 1)} onChange={(next) => updatePassField('min_score', next ?? 0)} disabled={disabled} /></label>
                  <label><span>最多入选</span><InputNumber size="small" min={0} value={numberValue(config.pass.top_n, 50)} onChange={(next) => updatePassField('top_n', next ?? 0)} disabled={disabled} /></label>
                  <label><span>周期</span><Select size="small" value={String(config.period || 'day')} options={[{ label: '日线', value: 'day' }, { label: '周线', value: 'week' }, { label: '月线', value: 'month' }]} onChange={(next) => updateConfigField('period', next)} disabled={disabled} /></label>
                  <label><span>计算根数</span><InputNumber size="small" min={20} value={numberValue(config.calc_count, 260)} onChange={(next) => updateConfigField('calc_count', next ?? 260)} disabled={disabled} /></label>
                  <label><span>批量大小</span><InputNumber size="small" min={1} value={numberValue(config.batch_size, 50)} onChange={(next) => updateConfigField('batch_size', next ?? 50)} disabled={disabled} /></label>
                  <label className="strategy-factor-runtime-switch"><span>错误后继续</span><Switch size="small" checked={Boolean(config.continue_on_error)} onChange={(next) => updateConfigField('continue_on_error', next)} disabled={disabled} /></label>
                  <label><span>次日最低开盘涨幅%</span><InputNumber size="small" value={config.execution.min_open_gap} placeholder="不限" onChange={(next) => updateExecutionField('min_open_gap', next)} disabled={disabled} /></label>
                  <label><span>次日最高开盘涨幅%</span><InputNumber size="small" value={config.execution.max_open_gap} placeholder="不限" onChange={(next) => updateExecutionField('max_open_gap', next)} disabled={disabled} /></label>
                </div>
              </section>
            </div>

            <aside className="strategy-factor-shelf">
              <div className="strategy-factor-shelf-head"><div><strong>因子货架</strong><span>点击下方选项加入规则</span></div><Tag>{factors.length}</Tag></div>
              <Input size="small" value={search} onChange={(event) => setSearch(event.target.value)} placeholder="搜索名称或 ID" allowClear disabled={disabled} />
              <div className="strategy-factor-shelf-list">
                {filteredFactors.map((factor) => (
                  <div className="strategy-factor-shelf-item" key={factor.id}>
                    <div><strong>{factor.name || factor.id}</strong><span>{factor.description || factor.id}</span></div>
                    <Space size={4}>
                      <Tooltip title="加入过滤"><Button aria-label={`将${factor.name}加入过滤`} type="text" size="small" icon={<FilterOutlined />} onClick={() => addRule('filters', factor)} disabled={disabled || factor.kind !== 'filter'} /></Tooltip>
                      <Tooltip title="加入评分"><Button aria-label={`将${factor.name}加入评分`} type="text" size="small" icon={<PlusOutlined />} onClick={() => addRule('scores', factor)} disabled={disabled || factor.kind !== 'score'} /></Tooltip>
                    </Space>
                  </div>
                ))}
                {!filteredFactors.length ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="没有匹配的因子" /> : null}
              </div>
            </aside>
          </div>
          <div className="strategy-factor-json-preview"><span>生成的配置</span><code>{formatJSON(config).slice(0, 520)}{formatJSON(config).length > 520 ? '…' : ''}</code></div>
        </div>
      )}
    </div>
  );
}
