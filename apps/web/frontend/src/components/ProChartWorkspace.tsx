import { Button, Card, Checkbox, Form, Input, Select, Space, Typography, message } from 'antd';
import { ClearOutlined, ExperimentOutlined, LineChartOutlined, ReloadOutlined, SettingOutlined } from '@ant-design/icons';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { apiFetch } from '../lib/api';
import { formatFormulaArgs, parseFormulaArgs, periodLabel } from '../lib/formula';
import { normalizeSymbol } from '../lib/format';
import type { Formula, FormulaRunResponse } from '../types';
import { FormulaManager } from './FormulaManager';

const { Text } = Typography;

const periodOptions = [
  { value: 'day', label: '日K' },
  { value: 'week', label: '周K' },
  { value: 'month', label: '月K' },
  { value: 'minute5', label: '5分钟' },
  { value: 'minute30', label: '30分钟' }
];

type ApplyMode = 'overlay' | 'change' | 'new-window';

export function ProChartWorkspace() {
  const chartRef = useRef<HTMLDivElement | null>(null);
  const [symbol, setSymbol] = useState('');
  const [period, setPeriod] = useState('day');
  const [formulas, setFormulas] = useState<Formula[]>([]);
  const [selectedFormulaID, setSelectedFormulaID] = useState('');
  const [loadingFormulas, setLoadingFormulas] = useState(false);
  const [chartStatus, setChartStatus] = useState('等待加载图表');
  const [testOutput, setTestOutput] = useState('');
  const [formulaDrawerOpen, setFormulaDrawerOpen] = useState(false);
  const [editingFormula, setEditingFormula] = useState<Formula>();
  const [applyMode, setApplyMode] = useState<ApplyMode>('overlay');
  const [windowIndex, setWindowIndex] = useState(0);
  const [independentY, setIndependentY] = useState(false);
  const [excludeY, setExcludeY] = useState(false);

  const selectedFormula = useMemo(
    () => formulas.find((item) => item.id === selectedFormulaID),
    [formulas, selectedFormulaID]
  );

  const loadFormulas = useCallback(async () => {
    setLoadingFormulas(true);
    try {
      const items = await apiFetch<Formula[]>('/api/formulas');
      setFormulas(items);
      setSelectedFormulaID((current) => current || items[0]?.id || '');
    } catch (error) {
      message.warning(error instanceof Error ? error.message : '公式列表加载失败');
    } finally {
      setLoadingFormulas(false);
    }
  }, []);

  const renderChart = useCallback(() => {
    const container = chartRef.current;
    if (!container) return;
    if (!normalizeSymbol(symbol)) {
      setChartStatus('请输入股票代码后加载图表');
      return;
    }
    const api = window.TDXHQChart;
    if (!api?.isAvailable?.()) {
      setChartStatus('HQChart 未加载，无法显示图表');
      return;
    }
    const ok = api.renderKLine(container, {
      symbol: normalizeSymbol(symbol),
      period,
      count: 800,
      pageSize: 80
    });
    setChartStatus(ok ? `${normalizeSymbol(symbol)} · ${periodLabel(period)} · 已加载` : 'HQChart 初始化失败');
  }, [period, symbol]);

  useEffect(() => {
    loadFormulas();
  }, [loadFormulas]);

  useEffect(() => {
    renderChart();
    const onResize = () => window.TDXHQChart?.resize?.(chartRef.current);
    window.addEventListener('resize', onResize);
    return () => {
      window.removeEventListener('resize', onResize);
      window.TDXHQChart?.destroy?.(chartRef.current);
    };
  }, [renderChart]);

  async function testFormula() {
    if (!selectedFormula) {
      message.warning('请先选择一个公式');
      return;
    }
    const normalizedSymbol = normalizeSymbol(symbol);
    if (!normalizedSymbol) {
      message.warning('请输入股票代码');
      return;
    }
    try {
      const data = await apiFetch<FormulaRunResponse>(`/api/formulas/${selectedFormula.id}/test`, {
        method: 'POST',
        body: JSON.stringify({
          symbol: normalizedSymbol,
          period,
          calc_count: 500,
          out_count: 20
        })
      });
      setTestOutput(JSON.stringify(data, null, 2));
      message.success(`测试完成 · ${data.engine || 'engine'} ${data.tick_ms || 0}ms`);
    } catch (error) {
      const text = error instanceof Error ? error.message : '公式测试失败';
      setTestOutput(text);
      message.error(text);
    }
  }

  async function applyFormula() {
    if (!selectedFormula) {
      message.warning('请先选择一个公式');
      return;
    }
    const container = chartRef.current;
    const chart = container ? window.TDXHQChart?.getChart?.(container) : null;
    const chartRecord = chart as Record<string, unknown> | null;
    if (!chartRecord) {
      renderChart();
      message.warning('图表初始化中，请稍后再应用公式');
      return;
    }
    try {
      const indexInfo = {
        Name: selectedFormula.name || '自定义公式',
        Script: selectedFormula.script,
        Args: parseFormulaArgs(selectedFormula),
        YAxis: { ExcludeValue: excludeY }
      };
      if (applyMode === 'change') {
        const changeScriptIndex = chartRecord.ChangeScriptIndex as undefined | ((index: number, info: unknown) => void);
        if (!changeScriptIndex) throw new Error('当前 HQChart 版本不支持 ChangeScriptIndex');
        changeScriptIndex(windowIndex, indexInfo);
      } else if (applyMode === 'new-window') {
        const addScriptIndexWindow = chartRecord.AddScriptIndexWindow as undefined | ((info: unknown, options: unknown) => void);
        if (!addScriptIndexWindow) throw new Error('当前 HQChart 版本不支持 AddScriptIndexWindow');
        addScriptIndexWindow(indexInfo, { Draw: true });
      } else {
        const addOverlayIndex = chartRecord.AddOverlayIndex as undefined | ((options: unknown) => void);
        if (!addOverlayIndex) throw new Error('当前 HQChart 版本不支持 AddOverlayIndex');
        addOverlayIndex({
          Script: indexInfo.Script,
          WindowIndex: windowIndex,
          Name: indexInfo.Name,
          Args: indexInfo.Args,
          IsShareY: !independentY,
          YAxis: independentY ? undefined : indexInfo.YAxis
        });
      }
      await testFormula();
      setChartStatus(`已应用：${selectedFormula.name}`);
    } catch (error) {
      message.error(error instanceof Error ? error.message : '公式应用失败');
    }
  }

  function clearOverlay() {
    setTestOutput('');
    renderChart();
    setChartStatus('未叠加公式');
  }

  return (
    <div className="pro-chart-layout">
      <Card
        className="work-card pro-chart-card"
        title="行情图表"
        extra={
          <Button type="primary" icon={<ReloadOutlined />} onClick={renderChart}>
            加载
          </Button>
        }
      >
        <Form layout="inline" className="chart-control-form">
          <Form.Item label="标的">
            <Input value={symbol} onChange={(event) => setSymbol(event.target.value)} placeholder="输入股票代码" />
          </Form.Item>
          <Form.Item label="周期">
            <Select value={period} options={periodOptions} onChange={setPeriod} className="period-select" />
          </Form.Item>
        </Form>
        <div ref={chartRef} className="hq-chart-surface" />
        <Text type="secondary" className="chart-status">
          {chartStatus}
        </Text>
      </Card>

      <Card
        className="work-card pro-formula-card"
        title="公式叠加"
        extra={
          <Button icon={<SettingOutlined />} onClick={() => setFormulaDrawerOpen(true)}>
            公式管理
          </Button>
        }
      >
        <Space direction="vertical" size={14} className="formula-panel">
          <Select
            value={selectedFormulaID || undefined}
            loading={loadingFormulas}
            placeholder="选择公式"
            options={formulas.map((formula) => ({ value: formula.id, label: formula.name }))}
            onChange={setSelectedFormulaID}
          />
          <Button
            block
            onClick={() => {
              if (!selectedFormula) {
                message.warning('请先选择一个公式');
                return;
              }
              setEditingFormula(selectedFormula);
              setFormulaDrawerOpen(true);
            }}
          >
            编辑所选
          </Button>
          <div className="form-grid two">
            <Select
              value={applyMode}
              onChange={setApplyMode}
              options={[
                { value: 'overlay', label: '叠加指标' },
                { value: 'change', label: '切换当前窗口' },
                { value: 'new-window', label: '新建副图' }
              ]}
            />
            <Select
              value={windowIndex}
              onChange={setWindowIndex}
              options={[
                { value: 0, label: '主图' },
                { value: 1, label: '副图1' }
              ]}
            />
          </div>
          <Checkbox checked={independentY} onChange={(event) => setIndependentY(event.target.checked)}>
            叠加指标使用独立坐标
          </Checkbox>
          <Checkbox checked={excludeY} onChange={(event) => setExcludeY(event.target.checked)}>
            指标不参与 Y 轴计算
          </Checkbox>
          <Text type="secondary">公式参数：{formatFormulaArgs(selectedFormula)}</Text>
          <Space wrap>
            <Button icon={<ExperimentOutlined />} onClick={testFormula}>
              测试当前股票
            </Button>
            <Button type="primary" icon={<LineChartOutlined />} onClick={applyFormula}>
              应用到 K 线
            </Button>
            <Button icon={<ClearOutlined />} onClick={clearOverlay}>
              清除叠加
            </Button>
          </Space>
          <pre className="formula-output">{testOutput || '测试结果会显示在这里'}</pre>
        </Space>
      </Card>

      <FormulaManager
        open={formulaDrawerOpen}
        formulas={formulas}
        loading={loadingFormulas}
        editingFormula={editingFormula}
        onClose={() => setFormulaDrawerOpen(false)}
        onEdit={setEditingFormula}
        onReload={loadFormulas}
      />
    </div>
  );
}
