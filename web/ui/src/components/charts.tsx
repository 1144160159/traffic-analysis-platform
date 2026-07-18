import { BarChart, CustomChart, GaugeChart, GraphChart, HeatmapChart, LineChart, LinesChart, PieChart, SankeyChart, ScatterChart } from 'echarts/charts';
import { GridComponent, LegendComponent, TitleComponent, TooltipComponent, VisualMapComponent } from 'echarts/components';
import * as echarts from 'echarts/core';
import type { ComposeOption } from 'echarts/core';
import { CanvasRenderer } from 'echarts/renderers';
import EChartsReactCore from 'echarts-for-react/esm/core';
import type { BarSeriesOption, CustomSeriesOption, GaugeSeriesOption, GraphSeriesOption, HeatmapSeriesOption, LineSeriesOption, LinesSeriesOption, PieSeriesOption, SankeySeriesOption, ScatterSeriesOption } from 'echarts/charts';
import type { GridComponentOption, LegendComponentOption, TitleComponentOption, TooltipComponentOption, VisualMapComponentOption } from 'echarts/components';
import type { KeyboardEvent } from 'react';

echarts.use([LineChart, GaugeChart, ScatterChart, HeatmapChart, LinesChart, CustomChart, PieChart, BarChart, SankeyChart, GraphChart, GridComponent, TooltipComponent, TitleComponent, LegendComponent, VisualMapComponent, CanvasRenderer]);

type ChartOption = ComposeOption<
  LineSeriesOption | GaugeSeriesOption | ScatterSeriesOption | HeatmapSeriesOption | LinesSeriesOption | CustomSeriesOption | PieSeriesOption | BarSeriesOption | SankeySeriesOption | GraphSeriesOption | GridComponentOption | TooltipComponentOption | TitleComponentOption | LegendComponentOption | VisualMapComponentOption
>;

export type WorldActivityPoint = {
  name: string;
  coord: [number, number];
  value: number;
  level?: 'low' | 'medium' | 'high';
  selected?: boolean;
};

export type WorldActivityFlow = {
  name: string;
  from: [number, number];
  to: [number, number];
  value: number;
  level?: 'low' | 'medium' | 'high';
  selected?: boolean;
};

export type CampaignDensityPoint = {
  name: string;
  x: number;
  y: number;
  value: number;
  level?: 'low' | 'medium' | 'high';
};

export type AbnormalImpactItem = {
  name: string;
  value: number;
  level?: 'low' | 'medium' | 'high';
};

export type EvidenceRingItem = {
  label: string;
  value: number;
  level?: 'low' | 'medium' | 'high';
};

export type AssetProtocolShareItem = {
  label: string;
  value: number;
};

export type AssetMetricRingItem = {
  label: string;
  value: number;
  max: number;
  color: string;
  suffix?: string;
};

export type AssetDistributionItem = {
  label: string;
  value: number;
  color?: string;
  detail?: string;
};

export type DashboardStageChartItem = {
  label: string;
  value: number;
  footnote?: string;
  level?: 'low' | 'medium' | 'high' | 'info';
  slaPercent?: number;
  pressurePercent?: number;
  action?: string;
};

export type DashboardTalkerChartItem = {
  label: string;
  value: number;
};

export type DashboardStageSlaBarsItem = {
  label: string;
  values: number[];
  level?: 'low' | 'medium' | 'high' | 'info';
};

export type DashboardKpiSparklineItem = {
  label: string;
  values: number[];
  level?: 'low' | 'medium' | 'high' | 'info';
};

export type DataQualityFieldTrendSeries = {
  name: string;
  color: string;
  values: number[];
};

export type DataQualityTrendSeries = {
  name: string;
  color: string;
  values: number[];
  type?: 'bar' | 'line';
  dashed?: boolean;
  area?: boolean;
};

export type DataQualityKpiSparklineTone = 'ok' | 'warn' | 'risk' | 'info';

export type DataQualityHeatmapRow = {
  label: string;
  values: DataQualityKpiSparklineTone[];
};

export type ExfilSankeyNode = {
  name: string;
  depth?: number;
};

export type ExfilSankeyLink = {
  source: string;
  target: string;
  value: number;
};

export type ExfilDistributionItem = {
  label: string;
  value: number;
  color?: string;
};

export type ExfilTrendPoint = {
  label: string;
  value: number;
};

export type ExfilStackedTrendSeries = {
  name: string;
  values: number[];
  color: string;
};

export type ExfilGraphNode = {
  name: string;
  x: number;
  y: number;
  value?: number;
  level?: 'low' | 'medium' | 'high';
  selected?: boolean;
};

export type ExfilGraphLink = {
  source: string;
  target: string;
  value: number;
  selected?: boolean;
};

export type TopicTopologyNode = {
  id: string;
  label: string;
  detail: string;
  x: number;
  y: number;
  tone: 'asset' | 'probe' | 'risk' | 'protocol' | 'proxy' | 'destination';
  size?: [number, number];
  selected?: boolean;
};

export type TopicTopologyLink = {
  source: string;
  target: string;
  tone?: 'info' | 'risk' | 'ok' | 'warn' | 'purple';
};

export type ExfilBarItem = {
  label: string;
  value: number;
};

export type EncryptedProtocolChartItem = {
  label: string;
  value: number;
  color: string;
};

export type EncryptedScatterChartPoint = {
  x: number;
  y: number;
  level: 'low' | 'medium' | 'high' | 'info';
};

const worldLandPaths = [
  'M13.8 65.0 L28.1 66.7 L19.6 71.5 L3.6 66.4 L0.0 69.5 L0.0 58.4 L13.8 65.0 Z',
  'M248.5 57.0 L257.4 63.3 L262.4 55.9 L270.5 56.5 L274.3 62.2 L241.2 77.7 L237.0 86.3 L243.6 91.4 L271.5 96.8 L278.0 107.8 L281.7 104.0 L278.3 98.1 L287.4 93.0 L281.9 86.7 L283.0 76.9 L294.9 76.5 L306.7 80.4 L312.1 88.3 L320.6 82.4 L345.3 105.1 L333.2 110.4 L315.6 110.5 L302.5 119.9 L319.3 113.2 L320.9 121.6 L333.9 122.4 L318.4 129.0 L316.2 126.5 L321.0 124.2 L313.5 124.6 L303.6 130.5 L305.7 134.3 L290.2 140.3 L289.1 146.6 L287.9 141.2 L289.6 151.2 L274.1 162.7 L276.7 180.0 L266.4 166.4 L231.7 171.4 L228.1 187.7 L232.5 196.3 L244.3 198.0 L249.2 191.7 L258.2 190.2 L253.0 205.9 L268.3 207.6 L267.2 219.2 L273.8 225.6 L286.6 226.0 L300.7 215.5 L300.8 224.8 L305.7 216.2 L310.6 220.7 L328.1 220.2 L341.3 233.4 L357.5 238.3 L360.0 250.2 L388.9 258.0 L401.1 264.3 L403.5 270.4 L392.6 286.3 L386.3 310.9 L367.6 319.1 L350.5 345.5 L337.7 344.2 L342.3 352.5 L319.1 364.1 L323.7 368.2 L313.1 376.5 L316.7 383.7 L307.9 390.9 L310.7 395.4 L302.8 399.5 L291.8 395.2 L290.0 385.2 L294.1 380.4 L289.9 379.6 L298.0 367.7 L293.5 370.1 L305.1 304.9 L288.9 290.7 L274.3 267.0 L278.4 257.4 L275.2 252.9 L285.8 239.3 L282.8 226.9 L279.0 225.2 L275.3 229.9 L262.1 222.4 L257.0 213.1 L212.5 199.2 L181.2 161.7 L196.0 185.6 L188.4 181.3 L174.2 158.2 L164.9 153.9 L154.4 138.0 L153.6 116.2 L159.5 119.2 L158.8 113.9 L146.0 108.8 L127.6 88.5 L91.3 80.9 L78.6 85.7 L81.6 79.8 L59.9 94.5 L42.3 98.9 L63.8 86.3 L50.1 87.0 L38.6 79.2 L53.4 70.0 L33.0 67.6 L50.9 66.3 L36.8 60.1 L65.1 51.8 L120.8 58.6 L144.1 54.2 L197.6 62.8 L205.1 58.9 L233.0 63.1 L238.2 58.1 L232.0 55.3 L235.5 50.2 L248.5 57.0 Z',
  'M156.9 115.2 L143.4 109.0 L156.9 115.2 Z',
  'M165.4 51.7 L150.2 50.4 L155.7 45.3 L153.0 43.6 L179.1 45.9 L165.4 51.7 Z',
  'M182.9 46.9 L199.5 51.0 L198.9 47.0 L204.1 47.0 L219.2 56.7 L185.2 59.6 L174.1 55.7 L187.7 54.5 L168.3 51.2 L182.9 46.9 Z',
  'M221.2 44.9 L229.5 45.1 L231.3 50.9 L215.3 48.6 L221.2 44.9 Z',
  'M237.0 35.8 L278.2 41.9 L250.7 43.0 L230.2 36.8 L237.0 35.8 Z',
  'M258.3 28.7 L261.6 29.6 L247.8 32.7 L231.4 27.3 L243.3 24.3 L258.3 28.7 Z',
  'M309.7 19.1 L328.2 20.5 L286.4 29.7 L290.6 31.9 L276.2 38.4 L251.4 37.6 L254.8 33.6 L264.0 34.6 L255.7 32.3 L263.6 29.6 L258.5 27.1 L272.6 26.5 L245.6 22.5 L309.7 19.1 Z',
  'M259.5 46.8 L299.3 51.2 L314.0 57.8 L308.9 59.1 L328.2 64.3 L322.4 69.4 L311.1 65.9 L320.4 73.9 L308.9 72.9 L316.2 78.0 L284.1 71.6 L283.6 68.6 L294.6 68.2 L297.4 61.9 L280.7 55.1 L250.3 52.2 L251.6 46.9 L259.5 46.8 Z',
  'M278.7 186.8 L293.9 193.7 L284.0 194.8 L272.8 187.1 L264.0 189.2 L278.7 186.8 Z',
  'M298.4 194.8 L310.2 198.3 L293.2 199.0 L299.1 198.1 L298.4 194.8 Z',
  'M311.8 399.6 L319.3 401.9 L307.7 404.2 L292.6 396.8 L302.5 400.2 L307.4 395.9 L311.8 399.6 Z',
  'M424.7 18.0 L442.1 20.2 L411.4 21.7 L466.1 24.2 L444.3 27.3 L450.7 27.4 L445.3 31.2 L448.7 36.2 L439.8 37.1 L446.2 43.6 L431.1 49.1 L439.6 53.7 L429.0 51.6 L426.8 54.9 L437.9 55.2 L389.4 68.2 L379.5 83.1 L365.9 80.9 L356.6 73.3 L350.1 63.4 L358.7 55.8 L348.1 56.6 L357.2 54.0 L344.9 51.0 L348.0 48.4 L337.3 40.2 L309.7 38.7 L301.7 36.1 L314.5 35.1 L296.4 33.2 L317.5 29.5 L311.0 27.5 L326.0 22.9 L424.7 18.0 Z',
  'M344.1 109.2 L342.2 111.6 L351.5 113.2 L352.6 120.4 L335.4 117.8 L346.1 106.7 L344.1 109.2 Z',
  'M459.7 65.4 L462.2 69.1 L448.2 73.6 L432.4 67.7 L459.7 65.4 Z',
  'M481.1 104.8 L472.3 106.1 L473.1 100.3 L481.3 96.7 L484.3 98.5 L481.1 104.8 Z',
  'M491.7 87.1 L488.7 90.1 L494.6 89.8 L491.3 94.5 L504.7 103.5 L504.0 107.5 L485.4 111.2 L490.5 107.1 L485.4 105.6 L487.3 101.4 L491.8 100.0 L482.9 92.3 L491.7 87.1 Z',
  'M550.7 28.6 L559.8 30.7 L544.2 36.7 L529.0 28.7 L550.7 28.6 Z',
  'M797.1 36.2 L817.0 39.3 L803.9 43.9 L852.7 45.7 L864.7 53.4 L888.5 51.4 L890.2 47.6 L971.0 59.2 L973.5 55.3 L988.1 55.9 L1000.0 58.4 L1000.0 69.5 L992.8 70.5 L997.9 76.9 L954.3 83.7 L950.3 97.6 L935.5 108.3 L933.1 92.3 L956.9 76.2 L944.8 81.8 L935.3 79.3 L930.7 85.7 L895.0 86.0 L875.4 98.0 L888.6 99.5 L892.7 104.9 L883.9 121.4 L854.3 139.6 L858.6 152.5 L851.3 154.5 L848.1 140.1 L836.3 142.0 L837.9 136.3 L827.9 141.1 L830.3 146.0 L839.9 146.0 L831.0 153.0 L838.6 162.0 L838.0 171.6 L821.9 186.7 L806.8 193.5 L801.5 189.7 L794.1 195.1 L803.7 212.7 L792.1 226.1 L778.0 212.8 L775.6 224.3 L786.0 234.7 L789.5 246.4 L781.6 242.3 L773.2 228.3 L769.9 203.0 L761.6 205.4 L753.9 186.8 L741.6 190.3 L723.1 205.8 L721.8 221.2 L715.4 227.9 L701.8 190.7 L695.8 192.0 L684.4 179.4 L659.4 178.5 L633.3 166.7 L643.9 183.3 L656.6 176.7 L666.1 188.0 L653.5 202.1 L620.8 214.9 L618.5 203.4 L597.0 168.1 L594.2 173.2 L590.1 167.1 L604.1 198.3 L618.7 217.4 L623.9 221.0 L642.0 216.6 L641.8 220.4 L632.6 238.3 L608.9 263.0 L613.3 290.8 L596.6 305.0 L598.9 315.9 L590.5 321.5 L589.5 329.9 L571.6 344.3 L551.0 344.8 L532.8 300.2 L538.0 279.8 L524.4 253.1 L526.1 239.6 L516.4 238.2 L512.0 232.6 L475.0 236.6 L453.9 216.2 L452.9 189.2 L483.5 150.7 L526.4 146.2 L530.8 147.5 L528.7 156.2 L553.0 165.9 L559.8 158.8 L593.8 164.0 L600.4 148.2 L576.8 148.2 L572.7 140.4 L593.1 133.3 L615.8 133.4 L601.9 124.3 L608.7 118.7 L597.1 121.5 L600.9 124.7 L594.1 126.8 L585.4 120.6 L576.9 131.7 L580.0 136.0 L562.9 138.2 L566.8 145.4 L562.5 148.9 L554.3 134.1 L536.5 123.0 L535.0 127.5 L551.3 138.4 L546.9 137.7 L544.7 144.5 L542.8 138.8 L524.7 126.8 L508.6 130.3 L494.0 148.1 L475.3 147.6 L473.9 130.5 L496.2 127.7 L496.7 122.2 L487.2 114.8 L495.5 114.9 L494.6 111.7 L522.6 101.3 L523.7 91.4 L529.4 89.6 L526.8 95.9 L530.4 100.0 L554.6 98.8 L559.9 90.5 L567.0 91.6 L564.8 85.6 L580.9 83.3 L559.2 81.3 L559.8 74.5 L570.6 69.1 L561.6 67.4 L549.6 75.7 L547.6 79.6 L552.2 83.1 L544.1 94.2 L536.0 96.2 L528.8 84.8 L515.7 87.3 L513.9 77.9 L541.0 61.6 L568.2 52.7 L614.1 62.6 L606.6 66.7 L592.2 64.9 L602.8 72.6 L603.3 69.0 L622.1 66.5 L620.7 59.5 L628.5 60.4 L628.7 64.8 L649.2 58.7 L666.5 60.3 L668.2 56.0 L690.3 60.9 L685.3 52.7 L701.6 47.8 L704.6 60.0 L698.0 65.8 L701.2 66.2 L708.5 61.8 L703.1 51.5 L707.4 47.7 L712.1 52.4 L726.4 50.7 L723.6 45.4 L742.1 41.3 L797.1 36.2 Z',
  'M639.0 287.7 L630.8 319.3 L622.3 319.4 L623.5 295.0 L636.7 283.4 L639.0 287.7 Z',
  'M659.8 53.6 L642.9 50.0 L654.5 41.4 L691.3 37.4 L662.4 43.6 L653.9 49.0 L659.8 53.6 Z',
  'M793.9 266.3 L785.0 261.7 L764.7 234.8 L788.4 249.7 L794.7 258.5 L793.9 266.3 Z',
  'M801.7 268.8 L821.4 273.3 L792.7 269.0 L801.7 268.8 Z',
  'M827.4 244.9 L830.5 247.5 L822.6 261.1 L806.2 258.2 L803.0 251.3 L804.6 244.4 L824.2 230.8 L831.1 235.0 L827.4 244.9 Z',
  'M837.0 198.6 L838.1 210.2 L844.7 215.2 L833.5 208.4 L837.0 198.6 Z',
  'M847.9 246.1 L833.8 249.3 L835.9 253.9 L842.6 251.7 L837.5 255.3 L842.1 264.8 L836.0 257.3 L831.6 264.9 L833.4 248.4 L847.9 246.1 Z',
  'M851.0 226.6 L848.3 234.5 L843.4 228.2 L838.7 230.0 L848.4 222.9 L851.0 226.6 Z',
  'M872.6 253.2 L876.3 259.4 L884.2 254.7 L901.6 260.7 L918.6 279.4 L902.1 271.2 L896.2 275.9 L882.3 273.4 L883.1 265.0 L869.4 261.4 L866.6 257.8 L871.4 256.2 L862.6 252.6 L872.6 253.2 Z',
  'M891.6 146.8 L889.6 152.4 L877.2 157.0 L863.9 155.9 L866.7 157.9 L861.7 162.7 L859.5 157.5 L876.9 151.3 L892.7 135.1 L891.6 146.8 Z',
  'M898.8 288.2 L925.4 322.4 L924.7 337.9 L916.7 354.0 L906.4 358.4 L890.7 355.6 L883.9 345.5 L880.1 347.9 L882.8 341.4 L877.7 346.9 L864.8 337.5 L827.8 347.4 L819.5 345.0 L821.4 337.8 L814.8 322.5 L817.1 310.4 L835.7 304.7 L849.1 289.5 L860.1 291.6 L867.7 280.9 L879.1 282.9 L876.4 291.7 L889.5 299.2 L895.9 279.6 L898.8 288.2 Z',
  'M899.8 127.3 L904.3 129.8 L888.8 134.5 L894.4 123.5 L899.8 127.3 Z',
  'M899.0 109.0 L901.8 114.0 L897.7 113.0 L898.6 121.8 L894.7 122.3 L895.0 99.4 L899.0 109.0 Z',
  'M903.9 363.3 L911.9 363.5 L910.9 370.0 L905.7 371.0 L903.9 363.3 Z',
  'M980.6 363.7 L984.0 364.9 L980.8 371.8 L970.4 379.6 L963.0 378.4 L980.6 363.7 Z',
  'M985.0 350.4 L995.9 354.7 L986.8 365.8 L979.5 345.9 L985.0 350.4 Z',
];

const worldLevelColor = (level: WorldActivityPoint['level'], variant: 'risk' | 'egress') => {
  if (variant === 'risk') {
    if (level === 'high') return '#ff4d4f';
    if (level === 'medium') return '#ffb020';
    return '#36d66b';
  }
  if (level === 'high') return '#ffb020';
  if (level === 'medium') return '#18a8ff';
  return '#7fd4ff';
};

export function WorldActivityMap({
  variant,
  points,
  flows = [],
  ariaLabel,
  onNodeClick,
}: {
  variant: 'risk' | 'egress';
  points: WorldActivityPoint[];
  flows?: WorldActivityFlow[];
  ariaLabel: string;
  onNodeClick?: (name: string) => void;
}) {
  const landFill = variant === 'risk' ? 'rgba(255, 77, 79, 0.24)' : 'rgba(24, 168, 255, 0.16)';
  const landStroke = variant === 'risk' ? 'rgba(255, 112, 82, 0.88)' : 'rgba(127, 212, 255, 0.62)';
  const glow = variant === 'risk' ? 'rgba(255, 77, 79, 0.58)' : 'rgba(24, 168, 255, 0.38)';
  const pointData = points.map((point) => ({
    name: point.name,
    value: [point.coord[0], point.coord[1], point.value],
    symbolSize: Math.max(6, Math.min(22, 5 + point.value / 6 + (point.selected ? 6 : 0))),
    itemStyle: {
      color: worldLevelColor(point.level, variant),
      borderColor: point.selected ? '#f4fbff' : 'rgba(234, 247, 255, 0.72)',
      borderWidth: point.selected ? 3 : 1,
      shadowBlur: point.selected ? 22 : 12,
      shadowColor: worldLevelColor(point.level, variant),
    },
    label: {
      show: Boolean(point.selected),
      formatter: point.name,
      position: 'top' as const,
      color: '#f4fbff',
      fontSize: 10,
      textBorderColor: '#03111c',
      textBorderWidth: 3,
    },
  }));
  const flowData = flows.map((flow) => ({
    name: flow.name,
    value: flow.value,
    coords: [flow.from, flow.to],
      lineStyle: {
        color: worldLevelColor(flow.level, 'egress'),
        width: Math.max(1.4, Math.min(4.5, 1.2 + flow.value / 20 + (flow.selected ? 1.3 : 0))),
        curveness: 0.28,
        opacity: flow.selected ? 1 : 0.72,
        shadowBlur: flow.selected ? 16 : 7,
      },
  }));
  const riskTraceData = variant === 'risk'
    ? [
        [[180, 212], [245, 196]],
        [[245, 196], [322, 226]],
        [[322, 226], [386, 204]],
        [[386, 204], [487, 178]],
        [[487, 178], [560, 214]],
        [[560, 214], [606, 236]],
        [[606, 236], [710, 212]],
        [[710, 212], [802, 238]],
        [[266, 248], [318, 334]],
        [[438, 222], [522, 285]],
        [[522, 285], [606, 236]],
        [[606, 236], [742, 213]],
        [[742, 213], [788, 270]],
        [[462, 170], [522, 285]],
        [[206, 206], [282, 224]],
        [[282, 224], [312, 338]],
        [[487, 178], [522, 285]],
        [[742, 213], [788, 270]],
      ].map((coords, index) => ({
        name: `风险传播纹理 ${index + 1}`,
        coords,
        lineStyle: {
          color: index % 3 === 0 ? 'rgba(255, 176, 32, 0.76)' : 'rgba(255, 77, 79, 0.72)',
          width: index % 4 === 0 ? 1.6 : 1,
          curveness: index % 2 === 0 ? 0.12 : -0.08,
          opacity: 0.82,
        },
      }))
    : [];

  const option: ChartOption = {
    backgroundColor: 'transparent',
    animation: false,
    tooltip: {
      trigger: 'item',
      confine: true,
      backgroundColor: 'rgba(3, 17, 28, 0.92)',
      borderColor: 'rgba(127, 212, 255, 0.24)',
      textStyle: { color: '#eaf7ff', fontSize: 11 },
      formatter: (rawParams) => {
        const params = Array.isArray(rawParams) ? rawParams[0] : rawParams;
        const item = params as { name?: string; value?: unknown };
        const value = Array.isArray(item.value) ? item.value[2] : item.value;
        return `${item.name ?? '图示节点'}<br/>强度：${value ?? '-'}`;
      },
    },
    grid: { left: 0, right: 0, top: 0, bottom: 0, containLabel: false },
    xAxis: {
      type: 'value',
      min: 0,
      max: 1000,
      show: false,
    },
    yAxis: {
      type: 'value',
      min: 0,
      max: 500,
      inverse: true,
      show: false,
    },
    series: [
      {
        name: 'world-land',
        type: 'custom',
        coordinateSystem: 'cartesian2d',
        silent: true,
        data: [0],
        renderItem: (_params, api) => {
          const [left, top] = api.coord([0, 0]);
          const size = api.size?.([1000, 500]);
          const width = Array.isArray(size) ? Math.abs(size[0]) : api.getWidth();
          const height = Array.isArray(size) ? Math.abs(size[1]) : api.getHeight();

          return {
            type: 'group',
            x: left,
            y: top,
            scaleX: width / 1000,
            scaleY: height / 500,
            children: worldLandPaths.map((pathData, index) => ({
              type: 'path',
              name: `land-${index}`,
              shape: { pathData },
              style: {
                fill: landFill,
                stroke: landStroke,
                lineWidth: 1.2,
                shadowBlur: 8,
                shadowColor: glow,
              },
              silent: true,
            })),
          };
        },
      },
      ...(riskTraceData.length
        ? [{
            name: '风险传播纹理',
            type: 'lines',
            coordinateSystem: 'cartesian2d',
            silent: true,
            z: 3,
            symbol: ['none', 'none'],
            data: riskTraceData,
            lineStyle: {
              width: 1,
              opacity: 0.76,
              shadowBlur: 7,
              shadowColor: 'rgba(255, 77, 79, 0.56)',
            },
          } as LinesSeriesOption]
        : []),
      {
        name: variant === 'risk' ? '风险热点' : '外联节点',
        type: 'scatter',
        coordinateSystem: 'cartesian2d',
        data: pointData,
        z: 5,
      },
      ...(flows.length
        ? [{
            name: '外联流向',
            type: 'lines',
            coordinateSystem: 'cartesian2d',
            data: flowData,
            z: 4,
            symbol: ['none', 'circle'],
            symbolSize: [0, 6],
            lineStyle: {
              color: '#18a8ff',
              width: 2,
              opacity: 0.86,
              curveness: 0.26,
              shadowBlur: 8,
              shadowColor: 'rgba(24, 168, 255, 0.72)',
            },
            effect: {
              show: true,
              constantSpeed: 28,
              symbolSize: 3,
              color: '#eaf7ff',
              trailLength: 0.18,
            },
          } as LinesSeriesOption]
        : []),
    ],
  };

  return (
    <EChartsReactCore
      aria-label={ariaLabel}
      echarts={echarts}
      style={{ height: '100%', width: '100%' }}
      option={option}
      notMerge
      lazyUpdate
      onEvents={onNodeClick ? {
        click: (params: { seriesType?: string; name?: string }) => {
          if (params.seriesType === 'scatter' && params.name) onNodeClick(params.name);
        },
      } : undefined}
    />
  );
}

export function CampaignDensityChart({
  points,
  ariaLabel,
}: {
  points: CampaignDensityPoint[];
  ariaLabel: string;
}) {
  const center: [number, number] = [50, 52];
  const outerRadius = 42;
  const clampToChart = (value: number) => Math.max(8, Math.min(92, value));
  const projectedPoints = points.map((point) => ({
    ...point,
    x: clampToChart(50 + (point.x - 50) * 0.94),
    y: clampToChart(52 + (point.y - 52) * 0.72),
  }));
  const expandedPoints = projectedPoints.flatMap((point, index) => {
    const satelliteCount = point.level === 'high' ? 4 : point.level === 'medium' ? 3 : 2;
    const radius = point.level === 'high' ? 8.5 : point.level === 'medium' ? 6.5 : 4.5;
    const seed = Array.from(point.name).reduce((sum, char) => sum + char.charCodeAt(0), index * 17);
    const satellites = Array.from({ length: satelliteCount }, (_, satelliteIndex) => {
      const angle = (seed * 0.017 + satelliteIndex * 2.19) % (Math.PI * 2);
      const offset = radius * (0.58 + satelliteIndex / (satelliteCount + 2));
      return {
        ...point,
        name: `${point.name} ${satelliteIndex + 1}`,
        x: clampToChart(point.x + Math.cos(angle) * offset),
        y: clampToChart(point.y + Math.sin(angle) * offset * 0.82),
        value: Math.max(6, point.value * (0.44 - satelliteIndex * 0.045)),
      };
    });
    return [point, ...satellites];
  });
  const heatmapStep = 2.5;
  const heatmapSigma = 10.5;
  const maxPointValue = Math.max(1, ...expandedPoints.map((point) => point.value));
  const heatmapData: Array<[number, number, number]> = [];

  for (let y = 14; y <= 96; y += heatmapStep) {
    for (let x = 10; x <= 90; x += heatmapStep) {
      const ringDistance = Math.hypot(x - center[0], y - center[1]);
      if (ringDistance > outerRadius + 4) continue;
      const density = expandedPoints.reduce((sum, point) => {
        const distanceSq = (x - point.x) ** 2 + (y - point.y) ** 2;
        const levelBoost = point.level === 'high' ? 1.28 : point.level === 'medium' ? 1.06 : 0.82;
        return sum + point.value * levelBoost * Math.exp(-distanceSq / (2 * heatmapSigma ** 2));
      }, 0);
      const normalized = Math.min(100, (density / maxPointValue) * 92);
      if (normalized > 1.2) heatmapData.push([Number(x.toFixed(1)), Number(y.toFixed(1)), Number(normalized.toFixed(2))]);
    }
  }

  const pointData = expandedPoints.map((point) => ({
    name: point.name,
    value: [point.x, point.y, point.value],
    symbolSize: Math.max(3.5, Math.min(8.8, 3.4 + point.value / 24)),
    itemStyle: {
      color: worldLevelColor(point.level, 'risk'),
      borderColor: 'rgba(234, 247, 255, 0.7)',
      borderWidth: 1,
      shadowBlur: point.level === 'high' ? 14 : 10,
      shadowColor: worldLevelColor(point.level, 'risk'),
    },
  }));

  const option: ChartOption = {
    backgroundColor: 'transparent',
    animation: false,
    tooltip: {
      trigger: 'item',
      confine: true,
      backgroundColor: 'rgba(3, 17, 28, 0.92)',
      borderColor: 'rgba(127, 212, 255, 0.24)',
      textStyle: { color: '#eaf7ff', fontSize: 11 },
      formatter: (rawParams) => {
        const params = Array.isArray(rawParams) ? rawParams[0] : rawParams;
        const item = params as { name?: string; value?: unknown };
        const value = Array.isArray(item.value) ? item.value[2] : item.value;
        return `${item.name ?? '战役簇'}<br/>密度：${value ?? '-'}`;
      },
    },
    visualMap: {
      show: false,
      min: 0,
      max: 100,
      seriesIndex: 1,
      inRange: {
        color: [
          'rgba(7, 28, 42, 0)',
          'rgba(24, 168, 255, 0.28)',
          'rgba(54, 214, 107, 0.46)',
          'rgba(255, 176, 32, 0.64)',
          'rgba(255, 91, 61, 0.86)',
        ],
      },
    },
    grid: { left: 2, right: 2, top: 2, bottom: 0, containLabel: false },
    xAxis: { type: 'value', min: 0, max: 100, show: false },
    yAxis: { type: 'value', min: 0, max: 100, inverse: true, show: false },
    series: [
      {
        name: '战役雷达网格',
        type: 'custom',
        coordinateSystem: 'cartesian2d',
        silent: true,
        data: [0],
        renderItem: (_params, api) => {
          const [cx, cy] = api.coord(center);
          const unitSize = api.size?.([1, 1]);
          const unit = Array.isArray(unitSize) ? Math.min(Math.abs(unitSize[0]), Math.abs(unitSize[1])) : 1;
          const radius = outerRadius * unit;
          const spokes = Array.from({ length: 12 }, (_, index) => {
            const angle = -Math.PI / 2 + (index * Math.PI * 2) / 12;
            return {
              type: 'line',
              shape: {
                x1: cx,
                y1: cy,
                x2: cx + Math.cos(angle) * radius,
                y2: cy + Math.sin(angle) * radius,
              },
              style: { stroke: 'rgba(127, 212, 255, 0.12)', lineWidth: 1 },
              silent: true,
            } as const;
          });
          return {
            type: 'group',
            children: [
              ...spokes,
              {
                type: 'circle',
                shape: { cx, cy, r: radius },
                style: { fill: 'rgba(24, 168, 255, 0.02)', stroke: 'rgba(127, 212, 255, 0.26)', lineWidth: 1, lineDash: [2, 4] },
              },
              {
                type: 'circle',
                shape: { cx, cy, r: radius * 0.68 },
                style: { fill: 'rgba(24, 168, 255, 0.018)', stroke: 'rgba(127, 212, 255, 0.2)', lineWidth: 1, lineDash: [2, 4] },
              },
              {
                type: 'circle',
                shape: { cx, cy, r: radius * 0.36 },
                style: { fill: 'rgba(255, 176, 32, 0.018)', stroke: 'rgba(255, 176, 32, 0.22)', lineWidth: 1, lineDash: [2, 4] },
              },
              {
                type: 'line',
                shape: { x1: cx - radius, y1: cy, x2: cx + radius, y2: cy },
                style: { stroke: 'rgba(127, 212, 255, 0.18)', lineWidth: 1 },
              },
              {
                type: 'line',
                shape: { x1: cx, y1: cy - radius, x2: cx, y2: cy + radius },
                style: { stroke: 'rgba(24, 168, 255, 0.28)', lineWidth: 1.2 },
              },
            ],
          };
        },
      },
      {
        name: '战役簇密度热力',
        type: 'heatmap',
        coordinateSystem: 'cartesian2d',
        data: heatmapData,
        silent: true,
        z: 2,
        emphasis: { disabled: true },
        itemStyle: {
          borderWidth: 0,
          opacity: 0.96,
        },
      },
      {
        name: '战役簇密度',
        type: 'scatter',
        coordinateSystem: 'cartesian2d',
        data: pointData,
        z: 5,
      },
    ],
  };

  return (
    <EChartsReactCore
      aria-label={ariaLabel}
      echarts={echarts}
      style={{ height: '100%', width: '100%' }}
      option={option}
      notMerge
      lazyUpdate
    />
  );
}

const exfilPalette = ['#18a8ff', '#65d86e', '#ffb020', '#b685ff', '#ff5b3d', '#7fd4ff'];

export function ExfilSankeyChart({
  nodes,
  links,
  ariaLabel,
}: {
  nodes: ExfilSankeyNode[];
  links: ExfilSankeyLink[];
  ariaLabel: string;
}) {
  const option: ChartOption = {
    backgroundColor: 'transparent',
    animation: false,
    tooltip: {
      trigger: 'item',
      confine: true,
      backgroundColor: 'rgba(3, 17, 28, 0.94)',
      borderColor: 'rgba(127, 212, 255, 0.26)',
      textStyle: { color: '#eaf7ff', fontSize: 11 },
      formatter: (rawParams) => {
        const params = Array.isArray(rawParams) ? rawParams[0] : rawParams;
        const item = params as { name?: string; value?: unknown; dataType?: string };
        if (item.dataType === 'edge') return `${item.name ?? '外传路径'}<br/>强度：${item.value ?? '-'}`;
        return `${item.name ?? '节点'}<br/>聚合量：${item.value ?? '-'}`;
      },
    },
    series: [
      {
        type: 'sankey',
        left: 4,
        right: 4,
        top: 8,
        bottom: 8,
        nodeWidth: 13,
        nodeGap: 7,
        draggable: false,
        emphasis: { focus: 'adjacency' },
        lineStyle: {
          color: 'gradient',
          opacity: 0.36,
          curveness: 0.52,
        },
        label: {
          color: '#d7f1ff',
          fontSize: 10,
          overflow: 'truncate',
          width: 76,
        },
        levels: exfilPalette.map((color, depth) => ({
          depth,
          itemStyle: {
            color,
            borderColor: 'rgba(234, 247, 255, 0.42)',
            borderWidth: 1,
          },
          lineStyle: {
            color,
            opacity: 0.28,
          },
        })),
        data: nodes.map((node, index) => ({
          name: node.name,
          depth: node.depth,
          itemStyle: {
            color: exfilPalette[(node.depth ?? index) % exfilPalette.length],
            borderColor: 'rgba(234, 247, 255, 0.42)',
            borderWidth: 1,
            shadowBlur: 8,
            shadowColor: exfilPalette[(node.depth ?? index) % exfilPalette.length],
          },
        })),
        links,
      },
    ],
  };

  return (
    <EChartsReactCore
      aria-label={ariaLabel}
      echarts={echarts}
      style={{ height: '100%', width: '100%' }}
      option={option}
      notMerge
      lazyUpdate
    />
  );
}

export function ExfilPieChart({
  items,
  ariaLabel,
  center = ['38%', '52%'],
  radius = ['46%', '72%'],
}: {
  items: ExfilDistributionItem[];
  ariaLabel: string;
  center?: [string, string];
  radius?: [string, string];
}) {
  const option: ChartOption = {
    backgroundColor: 'transparent',
    animation: false,
    tooltip: {
      trigger: 'item',
      confine: true,
      backgroundColor: 'rgba(3, 17, 28, 0.94)',
      borderColor: 'rgba(127, 212, 255, 0.26)',
      textStyle: { color: '#eaf7ff', fontSize: 11 },
      formatter: '{b}<br/>{c} ({d}%)',
    },
    series: [
      {
        type: 'pie',
        radius,
        center,
        label: { show: false },
        labelLine: { show: false },
        data: items.map((item, index) => ({
          name: item.label,
          value: item.value,
          itemStyle: { color: item.color ?? exfilPalette[index % exfilPalette.length] },
        })),
      },
    ],
  };

  return (
    <EChartsReactCore
      aria-label={ariaLabel}
      echarts={echarts}
      style={{ height: '100%', width: '100%' }}
      option={option}
      notMerge
      lazyUpdate
    />
  );
}

const assetProtocolPalette = ['#1688ff', '#27b8e6', '#4bc06a', '#d1c94a', '#ff9f2f', '#7a8ff5', '#8a9aaa'];

export function AssetTrafficProfileChart({
  inbound,
  outbound,
  eastWest,
  labels,
  ariaLabel,
}: {
  inbound: number[];
  outbound: number[];
  eastWest: number[];
  labels: string[];
  ariaLabel: string;
}) {
  const pointCount = Math.max(inbound.length, outbound.length, eastWest.length, 1);
  const normalizedLabels = Array.from({ length: pointCount }, (_, index) => labels[index] || `${String((index * 2 + 3) % 24).padStart(2, '0')}:00`);
  const normalize = (values: number[]) => Array.from({ length: pointCount }, (_, index) => values[index] ?? 0);
  const inboundValues = normalize(inbound);
  const outboundValues = normalize(outbound);
  const eastWestValues = normalize(eastWest);
  const peak = Math.max(1, ...inboundValues, ...outboundValues, ...eastWestValues);
  const axisMax = Math.max(100, Math.ceil(peak / 20) * 20);
  const option: ChartOption = {
    backgroundColor: 'transparent',
    animation: false,
    color: ['#1688ff', '#39c978', '#7a8ff5'],
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'line' },
      confine: true,
      backgroundColor: 'rgba(3, 17, 28, 0.96)',
      borderColor: 'rgba(127, 212, 255, 0.3)',
      textStyle: { color: '#eaf7ff', fontSize: 11 },
      valueFormatter: (value) => `${Number(value).toFixed(1)} Mbps`,
    },
    legend: {
      top: 0,
      right: 3,
      itemWidth: 8,
      itemHeight: 8,
      itemGap: 10,
      icon: 'rect',
      textStyle: { color: '#9db8c8', fontSize: 10 },
      data: ['入站', '出站', '东西向'],
    },
    grid: { left: 33, right: 5, top: 25, bottom: 22, containLabel: false },
    xAxis: {
      type: 'category',
      data: normalizedLabels,
      axisLine: { lineStyle: { color: 'rgba(127, 212, 255, 0.28)' } },
      axisTick: { show: false },
      axisLabel: {
        color: '#7f9dad',
        fontSize: 9,
        interval: 1,
        hideOverlap: true,
      },
    },
    yAxis: {
      type: 'value',
      min: 0,
      max: axisMax,
      interval: 20,
      name: 'Mbps',
      nameLocation: 'end',
      nameGap: 6,
      nameTextStyle: { color: '#9db8c8', fontSize: 10, align: 'right' },
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: { color: '#7f9dad', fontSize: 9 },
      splitLine: { lineStyle: { color: 'rgba(56, 151, 201, 0.15)', type: 'solid' } },
    },
    series: [
      { name: '入站', type: 'line', data: inboundValues, smooth: 0.28, showSymbol: false, lineStyle: { width: 2 }, areaStyle: { opacity: 0.13 } },
      { name: '出站', type: 'line', data: outboundValues, smooth: 0.28, showSymbol: false, lineStyle: { width: 2 }, areaStyle: { opacity: 0.1 } },
      { name: '东西向', type: 'line', data: eastWestValues, smooth: 0.28, showSymbol: false, lineStyle: { width: 1.6, type: 'dashed' }, areaStyle: { opacity: 0.05 } },
    ],
  };

  return (
    <div className="taf-asset-traffic" data-series-count="3" data-series-types="line,line,line" data-point-count={pointCount}>
      <EChartsReactCore aria-label={ariaLabel} echarts={echarts} style={{ height: '100%', width: '100%' }} option={option} notMerge lazyUpdate />
    </div>
  );
}

export function AssetProtocolDistributionChart({
  items,
  totalLabel,
  ariaLabel,
}: {
  items: AssetProtocolShareItem[];
  totalLabel: string;
  ariaLabel: string;
}) {
  const valuesByName = Object.fromEntries(items.map((item) => [item.label, item.value]));
  const option: ChartOption = {
    backgroundColor: 'transparent',
    animation: false,
    tooltip: {
      trigger: 'item',
      confine: true,
      backgroundColor: 'rgba(3, 17, 28, 0.96)',
      borderColor: 'rgba(127, 212, 255, 0.3)',
      textStyle: { color: '#eaf7ff', fontSize: 11 },
      formatter: '{b}<br/>{c}%',
    },
    title: {
      text: '总流量',
      subtext: totalLabel,
      left: '27%',
      top: '34%',
      textAlign: 'center',
      itemGap: 2,
      textStyle: { color: '#a9c4d3', fontSize: 10, fontWeight: 500 },
      subtextStyle: { color: '#eaf7ff', fontSize: 12, fontWeight: 700 },
    },
    legend: {
      orient: 'vertical',
      top: 'center',
      right: 2,
      itemWidth: 8,
      itemHeight: 8,
      itemGap: 5,
      icon: 'rect',
      selectedMode: false,
      data: items.map((item) => item.label),
      formatter: (name: string) => `{name|${name}} {value|${(valuesByName[name] ?? 0).toFixed(1)}%}`,
      textStyle: {
        color: '#9db8c8',
        fontSize: 9,
        rich: {
          name: { width: 67, color: '#9db8c8', fontSize: 9 },
          value: { width: 36, align: 'right', color: '#b8ccd8', fontSize: 9 },
        },
      },
    },
    series: [
      {
        name: '协议占比',
        type: 'pie',
      center: ['27%', '53%'],
      radius: ['42%', '64%'],
        startAngle: 90,
        clockwise: true,
        avoidLabelOverlap: true,
        label: { show: false },
        labelLine: { show: false },
        itemStyle: { borderColor: 'rgba(3, 17, 28, 0.78)', borderWidth: 1 },
        data: items.map((item, index) => ({
          name: item.label,
          value: item.value,
          itemStyle: { color: assetProtocolPalette[index % assetProtocolPalette.length] },
        })),
      },
    ],
  };

  return (
    <div className="taf-asset-protocol" data-protocol-count={items.length} data-total-label={totalLabel} data-chart-center="27%" data-chart-radius="42%-64%" data-legend-region="right">
      <EChartsReactCore aria-label={ariaLabel} echarts={echarts} style={{ height: '100%', width: '100%' }} option={option} notMerge lazyUpdate />
    </div>
  );
}

const assetDistributionPalette = ['#ff4d4f', '#ff8a34', '#f2c94c', '#39c978', '#1688ff', '#7a8ff5', '#27b8e6'];

export function AssetMetricRingsChart({ items, ariaLabel }: { items: AssetMetricRingItem[]; ariaLabel: string }) {
  const centers = items.map((_, index) => `${(index + 0.5) * (100 / Math.max(items.length, 1))}%`);
  const option: ChartOption = {
    backgroundColor: 'transparent',
    animation: false,
    tooltip: { show: false },
    series: items.map((item, index) => ({
      name: item.label,
      type: 'gauge',
      center: [centers[index], '40%'],
      radius: '50%',
      min: 0,
      max: Math.max(item.max, 1),
      startAngle: 90,
      endAngle: -269.9,
      pointer: { show: false },
      progress: { show: true, roundCap: true, width: 5, itemStyle: { color: item.color } },
      axisLine: { roundCap: true, lineStyle: { width: 5, color: [[1, 'rgba(91, 132, 154, 0.2)']] } },
      axisTick: { show: false },
      splitLine: { show: false },
      axisLabel: { show: false },
      anchor: { show: false },
      title: { show: true, offsetCenter: [0, '145%'], color: '#91adbd', fontSize: 10 },
      detail: {
        valueAnimation: false,
        offsetCenter: [0, '0%'],
        color: item.color,
        fontSize: 18,
        fontWeight: 700,
        formatter: `${item.value}${item.suffix ?? ''}`,
      },
      data: [{ value: item.value, name: item.label }],
    })),
  };
  return (
    <div className="taf-asset-metric-rings" data-metric-count={items.length} data-ring-radius="50%" data-title-offset="145%">
      <EChartsReactCore aria-label={ariaLabel} echarts={echarts} style={{ height: '100%', width: '100%' }} option={option} notMerge lazyUpdate />
    </div>
  );
}

export function AssetDistributionDonutChart({
  items,
  centerLabel,
  centerValue,
  ariaLabel,
  tone = 'standard',
}: {
  items: AssetDistributionItem[];
  centerLabel: string;
  centerValue: string;
  ariaLabel: string;
  tone?: 'standard' | 'risk';
}) {
  const total = Math.max(1, items.reduce((sum, item) => sum + item.value, 0));
  const valuesByName = Object.fromEntries(items.map((item) => [item.label, item]));
  const option: ChartOption = {
    backgroundColor: 'transparent',
    animation: false,
    tooltip: {
      trigger: 'item',
      confine: true,
      backgroundColor: 'rgba(3, 17, 28, 0.96)',
      borderColor: 'rgba(127, 212, 255, 0.3)',
      textStyle: { color: '#eaf7ff', fontSize: 11 },
      formatter: (rawParams) => {
        const params = rawParams as { name?: string; value?: number };
        const detail = valuesByName[params.name ?? '']?.detail;
        return `${params.name ?? '-'}<br/>${params.value ?? 0}${detail ? ` · ${detail}` : ''}`;
      },
    },
    title: {
      text: centerValue,
      subtext: centerLabel,
      left: '21%',
      top: '35%',
      textAlign: 'center',
      itemGap: 2,
      textStyle: { color: tone === 'risk' ? '#ff6b67' : '#eaf7ff', fontSize: 20, fontWeight: 700 },
      subtextStyle: { color: '#8ca9ba', fontSize: 10 },
    },
    legend: {
      orient: 'vertical',
      top: 'center',
      right: 4,
      itemWidth: 8,
      itemHeight: 8,
      itemGap: 6,
      icon: 'rect',
      selectedMode: false,
      data: items.map((item) => item.label),
      formatter: (name: string) => {
        const item = valuesByName[name];
        const percent = ((item?.value ?? 0) / total) * 100;
        return `{name|${name}} {value|${item?.detail || `${item?.value ?? 0}  ${percent.toFixed(1)}%`}}`;
      },
      textStyle: {
        color: '#9db8c8',
        fontSize: 9,
        rich: {
          name: { width: 54, color: '#9db8c8', fontSize: 9 },
          value: { width: 60, align: 'right', color: '#d7e8f1', fontSize: 9 },
        },
      },
    },
    series: [{
      name: centerLabel,
      type: 'pie',
      center: ['21%', '52%'],
      radius: ['40%', '58%'],
      startAngle: 90,
      clockwise: true,
      label: { show: false },
      labelLine: { show: false },
      itemStyle: { borderColor: 'rgba(3, 17, 28, 0.86)', borderWidth: 2 },
      data: items.map((item, index) => ({
        name: item.label,
        value: item.value,
        itemStyle: { color: item.color || assetDistributionPalette[index % assetDistributionPalette.length] },
      })),
    }],
  };
  return (
    <div className="taf-asset-distribution" data-segment-count={items.length} data-chart-center="21%" data-chart-radius="40%-58%" data-legend-region="right" data-legend-safe-gap="12">
      <EChartsReactCore aria-label={ariaLabel} echarts={echarts} style={{ height: '100%', width: '100%' }} option={option} notMerge lazyUpdate />
    </div>
  );
}

export function AssetDiscoveryActivityChart({
  labels,
  discovered,
  pending,
  ariaLabel,
}: {
  labels: string[];
  discovered: number[];
  pending: number[];
  ariaLabel: string;
}) {
  const pointCount = Math.max(labels.length, discovered.length, pending.length, 1);
  const xLabels = Array.from({ length: pointCount }, (_, index) => labels[index] || `${String(index * 2).padStart(2, '0')}:00`);
  const discoveredValues = Array.from({ length: pointCount }, (_, index) => discovered[index] ?? 0);
  const pendingValues = Array.from({ length: pointCount }, (_, index) => pending[index] ?? 0);
  const option: ChartOption = {
    backgroundColor: 'transparent',
    animation: false,
    color: ['#1688ff', '#ffb020'],
    tooltip: {
      trigger: 'axis',
      confine: true,
      backgroundColor: 'rgba(3, 17, 28, 0.96)',
      borderColor: 'rgba(127, 212, 255, 0.3)',
      textStyle: { color: '#eaf7ff', fontSize: 11 },
    },
    legend: { top: 0, right: 4, itemWidth: 8, itemHeight: 8, textStyle: { color: '#9db8c8', fontSize: 10 } },
    grid: { left: 34, right: 34, top: 27, bottom: 24 },
    xAxis: {
      type: 'category',
      data: xLabels,
      axisTick: { show: false },
      axisLine: { lineStyle: { color: 'rgba(127, 212, 255, 0.28)' } },
      axisLabel: { color: '#7f9dad', fontSize: 9, hideOverlap: true },
    },
    yAxis: [
      { type: 'value', min: 0, axisLine: { show: false }, axisTick: { show: false }, axisLabel: { color: '#7f9dad', fontSize: 9 }, splitLine: { lineStyle: { color: 'rgba(56, 151, 201, 0.15)' } } },
      { type: 'value', min: 0, max: 100, axisLine: { show: false }, axisTick: { show: false }, axisLabel: { color: '#7f9dad', fontSize: 9, formatter: '{value}%' }, splitLine: { show: false } },
    ],
    series: [
      { name: '发现资产', type: 'bar', data: discoveredValues, barMaxWidth: 16, itemStyle: { color: '#1688ff' } },
      { name: '待归属率', type: 'line', yAxisIndex: 1, data: pendingValues, smooth: true, symbolSize: 5, lineStyle: { width: 2, color: '#ffb020' }, itemStyle: { color: '#ffb020' } },
    ],
  };
  return (
    <div className="taf-asset-discovery-chart" data-point-count={pointCount}>
      <EChartsReactCore aria-label={ariaLabel} echarts={echarts} style={{ height: '100%', width: '100%' }} option={option} notMerge lazyUpdate />
    </div>
  );
}

export function AssetPeriodicHeatmapChart({ values, ariaLabel }: { values: number[]; ariaLabel: string }) {
  const days = ['周一', '周二', '周三', '周四', '周五', '周六', '周日'];
  const slots = values.length >= 7 ? Math.floor(values.length / 7) : 0;
  const labels = Array.from({ length: slots }, (_, index) => `${String(Math.round((24 / Math.max(slots, 1)) * index)).padStart(2, '0')}:00`);
  const data = days.flatMap((_, day) => Array.from({ length: slots }, (__, slot) => [slot, day, values[day * slots + slot] ?? 0]));
  const max = Math.max(1, ...values);
  const option: ChartOption = {
    backgroundColor: 'transparent',
    animation: false,
    tooltip: {
      position: 'top',
      confine: true,
      backgroundColor: 'rgba(3, 17, 28, 0.96)',
      borderColor: 'rgba(127, 212, 255, 0.3)',
      textStyle: { color: '#eaf7ff', fontSize: 11 },
      formatter: (rawParams) => {
        const params = rawParams as { value?: unknown[] };
        const value = Array.isArray(params.value) ? params.value : [];
        return `${days[Number(value[1])] ?? '-'} ${labels[Number(value[0])] ?? '-'}<br/>连接强度：${value[2] ?? 0}`;
      },
    },
    grid: { left: 38, right: 6, top: 4, bottom: 22 },
    xAxis: {
      type: 'category',
      data: labels,
      axisLine: { lineStyle: { color: 'rgba(127, 212, 255, 0.2)' } },
      axisTick: { show: false },
      axisLabel: { color: '#7f9dad', fontSize: 9, interval: Math.max(0, Math.floor(slots / 4) - 1) },
    },
    yAxis: {
      type: 'category',
      data: days,
      inverse: true,
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: { color: '#91adbd', fontSize: 10, interval: 0, margin: 8 },
    },
    visualMap: { show: false, min: 0, max, inRange: { color: ['#0b3551', '#17679c', '#29a8e8', '#75d1ff'] } },
    series: [{
      name: '周期性连接',
      type: 'heatmap',
      data,
      itemStyle: { borderColor: '#071a27', borderWidth: 2, borderRadius: 2 },
      emphasis: { itemStyle: { borderColor: '#eaf7ff', borderWidth: 1 } },
    }],
  };
  return (
    <div className="taf-asset-periodic-chart" data-slot-count={slots} data-day-count="7" data-y-axis-fill="monday-to-sunday">
      <EChartsReactCore aria-label={ariaLabel} echarts={echarts} style={{ height: '100%', width: '100%' }} option={option} notMerge lazyUpdate />
    </div>
  );
}

export function ExfilLineChart({
  points,
  ariaLabel,
}: {
  points: ExfilTrendPoint[];
  ariaLabel: string;
}) {
  const values = points.map((point) => point.value);
  const maxValue = Math.max(1, ...values);
  const option: ChartOption = {
    backgroundColor: 'transparent',
    animation: false,
    tooltip: {
      trigger: 'axis',
      confine: true,
      backgroundColor: 'rgba(3, 17, 28, 0.94)',
      borderColor: 'rgba(127, 212, 255, 0.26)',
      textStyle: { color: '#eaf7ff', fontSize: 11 },
    },
    grid: { left: 22, right: 8, top: 8, bottom: 18 },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: points.map((point) => point.label),
      axisLine: { lineStyle: { color: 'rgba(127, 212, 255, 0.28)' } },
      axisTick: { show: false },
      axisLabel: { color: '#8fb3c4', fontSize: 9, interval: Math.ceil(points.length / 4) },
    },
    yAxis: {
      type: 'value',
      min: 0,
      max: Math.ceil(maxValue * 1.16),
      splitLine: { lineStyle: { color: 'rgba(56, 151, 201, 0.13)' } },
      axisLabel: { color: '#8fb3c4', fontSize: 9 },
    },
    series: [
      {
        name: '上传峰值',
        type: 'line',
        smooth: true,
        symbol: 'circle',
        symbolSize: 4,
        data: values,
        lineStyle: { color: '#18d5ff', width: 2 },
        itemStyle: { color: '#18d5ff' },
        areaStyle: { color: 'rgba(24, 168, 255, 0.2)' },
      },
    ],
  };

  return (
    <EChartsReactCore
      aria-label={ariaLabel}
      echarts={echarts}
      style={{ height: '100%', width: '100%' }}
      option={option}
      notMerge
      lazyUpdate
    />
  );
}

export function PcapPacketTrendChart({
  points,
  ariaLabel,
}: {
  points: ExfilTrendPoint[];
  ariaLabel: string;
}) {
  const values = points.map((point) => point.value);
  const maxValue = Math.max(1, ...values);
  const option: ChartOption = {
    backgroundColor: 'transparent',
    animation: false,
    tooltip: {
      trigger: 'axis',
      confine: true,
      backgroundColor: 'rgba(3, 17, 28, 0.94)',
      borderColor: 'rgba(127, 212, 255, 0.26)',
      textStyle: { color: '#eaf7ff', fontSize: 11 },
    },
    grid: { left: 26, right: 8, top: 12, bottom: 20 },
    xAxis: {
      type: 'category',
      data: points.map((point) => point.label),
      axisLine: { lineStyle: { color: 'rgba(127, 212, 255, 0.28)' } },
      axisTick: { show: false },
      axisLabel: { color: '#8fb3c4', fontSize: 9, interval: Math.ceil(points.length / 4) },
    },
    yAxis: {
      type: 'value',
      min: 0,
      max: Math.ceil(maxValue * 1.16),
      splitLine: { lineStyle: { color: 'rgba(56, 151, 201, 0.13)' } },
      axisLabel: { color: '#8fb3c4', fontSize: 9 },
    },
    series: [
      {
        name: 'PCAP 字节',
        type: 'bar',
        barMaxWidth: 10,
        data: values,
        itemStyle: { color: '#18a8ff', borderRadius: [2, 2, 0, 0] },
      } as BarSeriesOption,
    ],
  };

  return (
    <EChartsReactCore
      aria-label={ariaLabel}
      echarts={echarts}
      style={{ height: '100%', width: '100%' }}
      option={option}
      notMerge
      lazyUpdate
    />
  );
}

export function EvidenceEntropyTrendChart({
  points,
  ariaLabel,
}: {
  points: ExfilTrendPoint[];
  ariaLabel: string;
}) {
  const option: ChartOption = {
    backgroundColor: 'transparent',
    animation: false,
    grid: { left: 4, right: 4, top: 6, bottom: 4 },
    tooltip: {
      trigger: 'axis',
      confine: true,
      backgroundColor: 'rgba(3, 17, 28, 0.94)',
      borderColor: 'rgba(127, 212, 255, 0.26)',
      textStyle: { color: '#eaf7ff', fontSize: 11 },
      formatter: '{b}<br/>熵分 {c}',
    },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: points.map((point) => point.label),
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: { show: false },
    },
    yAxis: {
      type: 'value',
      min: 0,
      max: 10,
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: { show: false },
      splitLine: { show: false },
    },
    series: [
      {
        name: '熵分',
        type: 'line',
        smooth: true,
        symbol: 'none',
        data: points.map((point) => point.value),
        lineStyle: { color: '#18a8ff', width: 2 },
        areaStyle: { color: 'rgba(24, 168, 255, 0.15)' },
      } as LineSeriesOption,
    ],
  };

  return <EChartsReactCore aria-label={ariaLabel} echarts={echarts} style={{ height: '100%', width: '100%' }} option={option} notMerge lazyUpdate />;
}

export function EncryptedProtocolTrendChart({
  items,
  trend,
  ariaLabel,
}: {
  items: EncryptedProtocolChartItem[];
  trend: number[];
  ariaLabel: string;
}) {
  const source = items.length ? items : [
    { label: 'TLS', value: 64, color: '#18a8ff' },
    { label: 'QUIC', value: 18, color: '#8d66ff' },
    { label: '未知加密', value: 18, color: '#ff4d4f' },
  ];
  const values = trend.length ? trend : Array.from({ length: 34 }, (_, index) => 18 + ((index * 13) % 44));
  const option: ChartOption = {
    backgroundColor: 'transparent',
    animation: false,
    tooltip: {
      trigger: 'axis',
      confine: true,
      backgroundColor: 'rgba(3, 17, 28, 0.94)',
      borderColor: 'rgba(127, 212, 255, 0.26)',
      textStyle: { color: '#eaf7ff', fontSize: 11 },
    },
    legend: {
      top: 2,
      left: 2,
      itemWidth: 8,
      itemHeight: 8,
      textStyle: { color: '#9db9c9', fontSize: 10 },
      data: source.map((item) => item.label),
    },
    grid: { left: '42%', right: 6, top: 28, bottom: 16 },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: values.map((_, index) => String(index + 1)),
      axisLine: { lineStyle: { color: 'rgba(127, 212, 255, 0.28)' } },
      axisTick: { show: false },
      axisLabel: { show: false },
    },
    yAxis: {
      type: 'value',
      min: 0,
      splitLine: { lineStyle: { color: 'rgba(56, 151, 201, 0.13)' } },
      axisLabel: { color: '#8fb3c4', fontSize: 9 },
    },
    series: [
      {
        name: '协议占比',
        type: 'pie',
        radius: ['34%', '58%'],
        center: ['20%', '58%'],
        label: { show: false },
        labelLine: { show: false },
        itemStyle: { borderColor: '#061827', borderWidth: 2 },
        data: source.map((item) => ({ name: item.label, value: item.value, itemStyle: { color: item.color } })),
      } as PieSeriesOption,
      {
        name: '加密流量趋势',
        type: 'line',
        data: values,
        smooth: true,
        symbol: 'none',
        lineStyle: { color: '#18d5ff', width: 2 },
        areaStyle: { color: 'rgba(24, 168, 255, 0.18)' },
        itemStyle: { color: '#18d5ff' },
      } as LineSeriesOption,
    ],
  };

  return <EChartsReactCore aria-label={ariaLabel} echarts={echarts} style={{ height: '100%', width: '100%' }} option={option} notMerge lazyUpdate />;
}

export function DataQualityFieldTrendChart({
  times,
  series,
  threshold,
  ariaLabel,
}: {
  times: string[];
  series: DataQualityFieldTrendSeries[];
  threshold: number;
  ariaLabel: string;
}) {
  const values = series.flatMap((item) => item.values);
  const maxValue = Math.max(threshold, ...values, 1);
  const option: ChartOption = {
    backgroundColor: 'transparent',
    animation: false,
    tooltip: {
      trigger: 'axis',
      confine: true,
      backgroundColor: 'rgba(3, 17, 28, 0.94)',
      borderColor: 'rgba(127, 212, 255, 0.26)',
      textStyle: { color: '#eaf7ff', fontSize: 11 },
    },
    grid: { left: 34, right: 16, top: 10, bottom: 8 },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: times,
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: { show: false },
      splitLine: { show: true, lineStyle: { color: 'rgba(56, 151, 201, 0.16)', type: 'dashed' } },
    },
    yAxis: {
      type: 'value',
      min: 0,
      max: Math.ceil(maxValue / 1000) * 1000,
      splitNumber: 4,
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: {
        color: '#8ca6b7',
        fontSize: 10,
        formatter: (value: number) => value === 0 ? '0' : `${Math.round(value / 1000)}k`,
      },
      splitLine: { lineStyle: { color: 'rgba(56, 151, 201, 0.16)', type: 'dashed' } },
    },
    series: series.map((item) => ({
      name: item.name,
      type: 'line',
      smooth: false,
      symbol: 'none',
      data: item.values,
      lineStyle: { color: item.color, width: 2.2 },
      itemStyle: { color: item.color },
      markLine: item === series[0]
        ? {
            silent: true,
            symbol: 'none',
            lineStyle: { color: '#ff4d4f', type: 'dashed', width: 1.2 },
            label: { show: true, formatter: `阈值线 ${threshold.toLocaleString()}`, color: '#ff7875', fontSize: 10, position: 'insideStartTop' },
            data: [{ yAxis: threshold }],
          }
        : undefined,
    })) as LineSeriesOption[],
  };

  return <EChartsReactCore aria-label={ariaLabel} className="taf-data-quality-field-trend-chart" echarts={echarts} style={{ height: '100%', width: '100%' }} option={option} notMerge lazyUpdate />;
}

export function DataQualityKpiSparklineChart({
  ariaLabel,
  className = 'taf-data-quality-field-kpi-echart',
  tone,
  values,
}: {
  ariaLabel: string;
  className?: string;
  tone: DataQualityKpiSparklineTone;
  values: number[];
}) {
  const color = tone === 'risk' ? '#ff4d4f' : tone === 'warn' ? '#ffb020' : tone === 'info' ? '#18a8ff' : '#36d66b';
  const source = values.length ? values : [42, 46, 43, 49, 45, 51, 48, 52];
  const min = Math.min(...source);
  const max = Math.max(...source);
  const padding = Math.max((max - min) * 0.12, 1);
  const option: ChartOption = {
    backgroundColor: 'transparent',
    animation: false,
    tooltip: {
      trigger: 'axis',
      confine: true,
      backgroundColor: 'rgba(3, 17, 28, 0.94)',
      borderColor: 'rgba(127, 212, 255, 0.26)',
      textStyle: { color: '#eaf7ff', fontSize: 10 },
    },
    grid: { left: 1, right: 1, top: 1, bottom: 1 },
    xAxis: { type: 'category', boundaryGap: false, data: source.map((_, index) => `${index + 1}`), show: false },
    yAxis: { type: 'value', min: min - padding, max: max + padding, show: false },
    series: [{
      name: ariaLabel,
      type: 'line',
      data: source,
      smooth: false,
      symbol: 'none',
      lineStyle: { color, width: 1.7, shadowBlur: 5, shadowColor: `${color}88` },
      areaStyle: { color: `${color}14` },
      itemStyle: { color },
    } as LineSeriesOption],
  };

  return <EChartsReactCore aria-label={ariaLabel} className={className} echarts={echarts} style={{ height: '100%', width: '100%' }} option={option} notMerge lazyUpdate />;
}

export function ForensicsSessionTimelineChart({
  ariaLabel,
  rows,
}: {
  ariaLabel: string;
  rows: Array<{ time: string; protocol: string; packetCount: number }>;
}) {
  const source = rows.length ? rows : [{ time: '-', protocol: '其他', packetCount: 0 }];
  const labelInterval = Math.max(0, Math.ceil(source.length / 6) - 1);
  const packetCeiling = Math.max(...source.map((item) => Math.log10(Math.max(item.packetCount, 0) + 1)), 1) * 1.4;
  const protocols = [
    { name: 'TLS', color: '#18a8ff' },
    { name: 'HTTP', color: '#36d66b' },
    { name: 'DNS', color: '#ffb020' },
    { name: '其他', color: '#6f8796' },
  ];
  const option: ChartOption = {
    backgroundColor: 'transparent',
    animation: false,
    tooltip: {
      trigger: 'axis',
      confine: true,
      backgroundColor: 'rgba(3, 17, 28, 0.94)',
      borderColor: 'rgba(127, 212, 255, 0.26)',
      textStyle: { color: '#eaf7ff', fontSize: 10 },
    },
    legend: {
      bottom: 0,
      itemWidth: 8,
      itemHeight: 6,
      itemGap: 14,
      textStyle: { color: '#8ca6b7', fontSize: 9 },
    },
    grid: { left: 4, right: 4, top: 4, bottom: 22 },
    xAxis: {
      type: 'category',
      data: source.map((_, index) => String(index)),
      axisLine: { lineStyle: { color: 'rgba(56, 151, 201, 0.22)' } },
      axisTick: { show: false },
      axisLabel: { color: '#6f8796', fontSize: 9, interval: labelInterval, formatter: (value: string) => source[Number(value)]?.time ?? '' },
    },
    yAxis: { type: 'value', min: 0, max: packetCeiling, show: false },
    series: protocols.map((protocol) => ({
      name: protocol.name,
      type: 'bar',
      stack: 'session-packets',
      barMaxWidth: 7,
      data: source.map((item) => {
        const name = ['TLS', 'HTTP', 'DNS'].includes(item.protocol.toUpperCase()) ? item.protocol.toUpperCase() : '其他';
        return name === protocol.name ? Math.log10(Math.max(item.packetCount, 0) + 1) : 0;
      }),
      itemStyle: { color: protocol.color, borderRadius: [2, 2, 0, 0] },
    } as BarSeriesOption)),
  };

  return <EChartsReactCore aria-label={ariaLabel} echarts={echarts} style={{ height: '100%', width: '100%' }} option={option} notMerge lazyUpdate />;
}

export function DataQualityTrendChart({
  ariaLabel,
  categories,
  className,
  series,
  valueFormatter,
}: {
  ariaLabel: string;
  categories: string[];
  className?: string;
  series: DataQualityTrendSeries[];
  valueFormatter?: (value: number) => string;
}) {
  const maxLength = Math.max(...series.map((item) => item.values.length), 1);
  const axisLabels = Array.from({ length: maxLength }, (_, index) => categories[index % Math.max(categories.length, 1)] ?? `${index + 1}`);
  const values = series.flatMap((item) => item.values);
  const dataMax = values.length ? Math.max(...values) : 1;
  const dataMin = values.length ? Math.min(...values) : 0;
  const max = dataMax > 0 ? dataMax * 1.12 : 0;
  const min = dataMin < 0 ? dataMin * 1.12 : 0;
  const option: ChartOption = {
    backgroundColor: 'transparent',
    animation: false,
    tooltip: {
      trigger: 'axis',
      confine: true,
      backgroundColor: 'rgba(3, 17, 28, 0.94)',
      borderColor: 'rgba(127, 212, 255, 0.26)',
      textStyle: { color: '#eaf7ff', fontSize: 11 },
    },
    legend: {
      top: 0,
      right: 4,
      itemWidth: 10,
      itemHeight: 2,
      textStyle: { color: '#a8c0cf', fontSize: 10 },
    },
    grid: { left: 34, right: 14, top: 30, bottom: 24 },
    xAxis: {
      type: 'category',
      boundaryGap: series.some((item) => item.type === 'bar'),
      data: axisLabels,
      axisLine: { lineStyle: { color: 'rgba(56, 151, 201, 0.26)' } },
      axisTick: { show: false },
      axisLabel: { color: '#8ca6b7', fontSize: 9, interval: Math.max(0, Math.floor(axisLabels.length / 6) - 1) },
    },
    yAxis: {
      type: 'value',
      min,
      max: max === min ? min + 1 : max,
      splitNumber: 4,
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: { color: '#8ca6b7', fontSize: 9, formatter: valueFormatter },
      splitLine: { lineStyle: { color: 'rgba(56, 151, 201, 0.16)', type: 'dashed' } },
    },
    series: series.map((item) => ({
      name: item.name,
      type: item.type ?? 'line',
      data: item.values,
      smooth: false,
      symbol: item.type === 'bar' ? 'none' : 'none',
      barMaxWidth: item.type === 'bar' ? 18 : undefined,
      lineStyle: item.type === 'bar' ? undefined : { color: item.color, width: 2, type: item.dashed ? 'dashed' : 'solid' },
      itemStyle: { color: item.color },
      areaStyle: item.area ? { color: `${item.color}18` } : undefined,
      emphasis: { disabled: true },
    })) as Array<LineSeriesOption | BarSeriesOption>,
  };

  return <EChartsReactCore aria-label={ariaLabel} className={className} echarts={echarts} style={{ height: '100%', width: '100%' }} option={option} notMerge lazyUpdate />;
}

export function DataQualityHeatmapChart({
  ariaLabel,
  cellLabels,
  className,
  mode = 'skew',
  rows,
  times,
}: {
  ariaLabel: string;
  cellLabels?: string[][];
  className?: string;
  mode?: 'skew' | 'field-quality' | 'backpressure';
  rows: DataQualityHeatmapRow[];
  times: string[];
}) {
  const toneValue: Record<DataQualityKpiSparklineTone, number> = { ok: 0, info: 0.58, warn: 1.22, risk: 2 };
  const toneLabel = (value: number) => {
    if (mode === 'field-quality') return value >= 1.7 ? '较差' : value >= 0.9 ? '中等' : value >= 0.2 ? '不适用' : '优秀';
    if (mode === 'backpressure') return value >= 1.7 ? '严重背压' : value >= 0.9 ? '轻度背压' : '空闲';
    return value >= 1.7 ? '严重倾斜' : value >= 0.9 ? '轻度倾斜' : value >= 0.2 ? '需关注' : '均衡';
  };
  const visualPieces = mode === 'field-quality'
    ? [
        { min: 0, max: 0.15, label: '优秀 (>=98%)', color: '#29924f' },
        { min: 0.16, max: 0.85, label: '不适用', color: '#526574' },
        { min: 0.86, max: 1.6, label: '中等 (95%-98%)', color: '#c4870a' },
        { min: 1.61, max: 2.1, label: '较差 (<95%)', color: '#cf4450' },
      ]
    : mode === 'backpressure'
      ? [
          { min: 0, max: 0.85, label: '空闲 (0-0.1)', color: '#23a566' },
          { min: 0.86, max: 1.6, label: '轻度背压 (0.1-0.5)', color: '#ffb020' },
          { min: 1.61, max: 2.1, label: '严重背压 (>0.5)', color: '#ff4d4f' },
        ]
      : [
          { min: 0, max: 0.15, label: '均衡', color: '#237b9b' },
          { min: 0.16, max: 0.85, label: '需关注', color: '#3e9bb4' },
          { min: 0.86, max: 1.6, label: '轻度倾斜', color: '#c58a2a' },
          { min: 1.61, max: 2.1, label: '严重倾斜', color: '#c94c5d' },
        ];
  const columnCount = Math.max(...rows.map((row) => row.values.length), 1);
  const categories = Array.from({ length: columnCount }, (_, index) => String(index));
  const values = rows.flatMap((row, rowIndex) => row.values.map((tone, columnIndex) => [columnIndex, rowIndex, toneValue[tone]] as [number, number, number]));
  const option: ChartOption = {
    backgroundColor: 'transparent',
    animation: false,
    tooltip: {
      trigger: 'item',
      confine: true,
      backgroundColor: 'rgba(3, 17, 28, 0.96)',
      borderColor: 'rgba(127, 212, 255, 0.3)',
      textStyle: { color: '#eaf7ff', fontSize: 11 },
      formatter: (params: unknown) => {
        const value = (params as { value?: unknown }).value;
        if (!Array.isArray(value)) return '';
        const columnIndex = Number(value[0] ?? 0);
        const rowIndex = Number(value[1] ?? 0);
        const level = Number(value[2] ?? 0);
        const time = times[Math.round(columnIndex * Math.max(times.length - 1, 0) / Math.max(columnCount - 1, 1))] ?? '--';
        return `${rows[rowIndex]?.label ?? '--'}<br/>${time} · ${toneLabel(level)}`;
      },
    },
    grid: { left: mode === 'backpressure' ? 112 : mode === 'field-quality' ? 92 : 54, right: 16, top: 14, bottom: 58, containLabel: false },
    xAxis: {
      type: 'category',
      data: categories,
      splitArea: { show: false },
      axisLine: { lineStyle: { color: 'rgba(83, 188, 241, 0.34)' } },
      axisTick: { show: false },
      axisLabel: {
        color: '#a7c2d2',
        fontSize: 10,
        margin: 10,
        interval: Math.max(0, Math.ceil(columnCount / 7) - 1),
        formatter: (_value: string, index: number) => times[Math.round(index * Math.max(times.length - 1, 0) / Math.max(columnCount - 1, 1))] ?? '',
      },
    },
    yAxis: {
      type: 'category',
      data: rows.map((row) => row.label),
      inverse: true,
      splitArea: { show: false },
      axisLine: { lineStyle: { color: 'rgba(83, 188, 241, 0.34)' } },
      axisTick: { show: false },
      axisLabel: { color: '#bdd4e0', fontSize: 10, margin: 11 },
    },
    visualMap: {
      type: 'piecewise',
      orient: 'horizontal',
      left: 'center',
      bottom: 6,
      itemWidth: 14,
      itemHeight: 9,
      itemGap: 12,
      textStyle: { color: '#b8ceda', fontSize: 9 },
      pieces: visualPieces,
    } as VisualMapComponentOption,
    series: [{
      name: mode === 'field-quality' ? '字段质量' : mode === 'backpressure' ? 'Backpressure' : '分区倾斜',
      type: 'heatmap',
      data: values,
      label: cellLabels ? {
        show: true,
        color: '#f1fbff',
        fontSize: 10,
        fontWeight: 650,
        formatter: (params: unknown) => {
          const value = (params as { value?: unknown }).value;
          if (!Array.isArray(value)) return '';
          return cellLabels[Number(value[1])]?.[Number(value[0])] ?? '';
        },
      } : undefined,
      itemStyle: { borderColor: 'rgba(3, 17, 28, 0.9)', borderWidth: 2, borderRadius: 4, shadowBlur: 3, shadowColor: 'rgba(0, 0, 0, 0.28)' },
      emphasis: { itemStyle: { borderColor: '#d9f4ff', borderWidth: 1, shadowBlur: 12, shadowColor: 'rgba(24, 168, 255, 0.58)' } },
    } as HeatmapSeriesOption],
  };

  return <EChartsReactCore aria-label={ariaLabel} className={className} echarts={echarts} style={{ height: '100%', width: '100%' }} option={option} notMerge lazyUpdate />;
}

export function DataQualityDonutChart({
  ariaLabel,
  className,
  rows,
}: {
  ariaLabel: string;
  className?: string;
  rows: Array<{ label: string; value: number; color: string }>;
}) {
  const option: ChartOption = {
    backgroundColor: 'transparent',
    animation: false,
    tooltip: {
      trigger: 'item',
      confine: true,
      backgroundColor: 'rgba(3, 17, 28, 0.94)',
      borderColor: 'rgba(127, 212, 255, 0.26)',
      textStyle: { color: '#eaf7ff', fontSize: 11 },
    },
    series: [{
      name: ariaLabel,
      type: 'pie',
      radius: ['56%', '76%'],
      center: ['50%', '50%'],
      label: { show: false },
      labelLine: { show: false },
      data: rows.map((row) => ({ name: row.label, value: row.value, itemStyle: { color: row.color } })),
      emphasis: { scale: false },
    } as PieSeriesOption],
  };

  return <EChartsReactCore aria-label={ariaLabel} className={className} echarts={echarts} style={{ height: '100%', width: '100%' }} option={option} notMerge lazyUpdate />;
}

export function EncryptedJa3ScatterChart({
  points,
  ariaLabel,
}: {
  points: EncryptedScatterChartPoint[];
  ariaLabel: string;
}) {
  const palette: Record<EncryptedScatterChartPoint['level'], string> = {
    high: '#ff4d4f',
    medium: '#ffb020',
    low: '#36d66b',
    info: '#18a8ff',
  };
  const levels: EncryptedScatterChartPoint['level'][] = ['high', 'medium', 'low', 'info'];
  const option: ChartOption = {
    backgroundColor: 'transparent',
    animation: false,
    tooltip: {
      trigger: 'item',
      formatter: '{a}<br/>流量 {c[0]}<br/>会话 {c[1]}',
      backgroundColor: 'rgba(3, 17, 28, 0.94)',
      borderColor: 'rgba(127, 212, 255, 0.26)',
      textStyle: { color: '#eaf7ff', fontSize: 11 },
    },
    legend: { top: 2, right: 4, itemWidth: 8, itemHeight: 8, textStyle: { color: '#9db9c9', fontSize: 10 } },
    grid: { left: 34, right: 12, top: 25, bottom: 23 },
    xAxis: {
      type: 'value', min: 0, max: 100, name: '流量', nameTextStyle: { color: '#7f9cad', fontSize: 10 },
      axisLine: { lineStyle: { color: 'rgba(127, 212, 255, 0.28)' } }, axisTick: { show: false }, axisLabel: { color: '#8fb3c4', fontSize: 9 }, splitLine: { lineStyle: { color: 'rgba(56, 151, 201, 0.13)' } },
    },
    yAxis: {
      type: 'value', min: 0, max: 100, name: '会话', nameTextStyle: { color: '#7f9cad', fontSize: 10 },
      axisLine: { lineStyle: { color: 'rgba(127, 212, 255, 0.28)' } }, axisTick: { show: false }, axisLabel: { color: '#8fb3c4', fontSize: 9 }, splitLine: { lineStyle: { color: 'rgba(56, 151, 201, 0.13)' } },
    },
    series: levels.map((level) => ({
      name: level === 'high' ? '高危' : level === 'medium' ? '中危' : level === 'low' ? '低危' : '未知',
      type: 'scatter',
      symbolSize: 8,
      data: points.filter((point) => point.level === level).map((point) => [point.x, point.y]),
      itemStyle: { color: palette[level], shadowBlur: 8, shadowColor: palette[level] },
    })) as ScatterSeriesOption[],
  };

  return <EChartsReactCore aria-label={ariaLabel} echarts={echarts} style={{ height: '100%', width: '100%' }} option={option} notMerge lazyUpdate />;
}

export function HeartbeatTrendChart({
  values,
  ariaLabel,
}: {
  values: number[];
  ariaLabel: string;
}) {
  const option: ChartOption = {
    backgroundColor: 'transparent',
    animation: false,
    tooltip: {
      trigger: 'axis', confine: true, backgroundColor: 'rgba(3, 17, 28, 0.94)', borderColor: 'rgba(127, 212, 255, 0.26)', textStyle: { color: '#eaf7ff', fontSize: 11 },
    },
    grid: { left: 26, right: 8, top: 10, bottom: 20 },
    xAxis: {
      type: 'category', boundaryGap: false, data: values.map((_, index) => String(index + 1)),
      axisLine: { lineStyle: { color: 'rgba(127, 212, 255, 0.28)' } }, axisTick: { show: false }, axisLabel: { color: '#8fb3c4', fontSize: 9, interval: Math.ceil(values.length / 5) },
    },
    yAxis: {
      type: 'value', min: 0, splitLine: { lineStyle: { color: 'rgba(56, 151, 201, 0.13)' } }, axisLabel: { color: '#8fb3c4', fontSize: 9 },
    },
    series: [{
      name: '心跳间隔', type: 'line', data: values, smooth: true, symbol: 'none', lineStyle: { color: '#36d66b', width: 2 }, areaStyle: { color: 'rgba(54, 214, 107, 0.16)' }, itemStyle: { color: '#36d66b' },
    } as LineSeriesOption],
  };

  return <EChartsReactCore aria-label={ariaLabel} echarts={echarts} style={{ height: '100%', width: '100%' }} option={option} notMerge lazyUpdate />;
}

export function ExfilStackedTrendChart({
  labels,
  series,
  ariaLabel,
}: {
  labels: string[];
  series: ExfilStackedTrendSeries[];
  ariaLabel: string;
}) {
  const option: ChartOption = {
    backgroundColor: 'transparent',
    animation: false,
    tooltip: {
      trigger: 'axis',
      confine: true,
      backgroundColor: 'rgba(3, 17, 28, 0.94)',
      borderColor: 'rgba(127, 212, 255, 0.26)',
      textStyle: { color: '#eaf7ff', fontSize: 11 },
    },
    legend: {
      top: 0,
      right: 0,
      itemWidth: 7,
      itemHeight: 7,
      textStyle: { color: '#8fb3c4', fontSize: 9 },
      selectedMode: false,
    },
    grid: { left: 28, right: 8, top: 25, bottom: 19 },
    xAxis: {
      type: 'category',
      data: labels,
      axisLine: { lineStyle: { color: 'rgba(127, 212, 255, 0.28)' } },
      axisTick: { show: false },
      axisLabel: { color: '#8fb3c4', fontSize: 9, interval: Math.ceil(labels.length / 5) },
    },
    yAxis: {
      type: 'value',
      min: 0,
      splitLine: { lineStyle: { color: 'rgba(56, 151, 201, 0.13)' } },
      axisLabel: { color: '#8fb3c4', fontSize: 9 },
    },
    series: series.map((item) => ({
      name: item.name,
      type: 'bar',
      stack: 'egress-events',
      barMaxWidth: 11,
      emphasis: { focus: 'series' },
      data: item.values,
      itemStyle: { color: item.color },
    } as BarSeriesOption)),
  };

  return (
    <EChartsReactCore
      aria-label={ariaLabel}
      echarts={echarts}
      style={{ height: '100%', width: '100%' }}
      option={option}
      notMerge
      lazyUpdate
    />
  );
}

export function ExfilGraphChart({
  nodes,
  links,
  ariaLabel,
  onNodeClick,
}: {
  nodes: ExfilGraphNode[];
  links: ExfilGraphLink[];
  ariaLabel: string;
  onNodeClick?: (name: string) => void;
}) {
  const option: ChartOption = {
    backgroundColor: 'transparent',
    animation: false,
    tooltip: {
      trigger: 'item',
      confine: true,
      backgroundColor: 'rgba(3, 17, 28, 0.94)',
      borderColor: 'rgba(127, 212, 255, 0.26)',
      textStyle: { color: '#eaf7ff', fontSize: 11 },
      formatter: (rawParams) => {
        const params = Array.isArray(rawParams) ? rawParams[0] : rawParams;
        const item = params as { dataType?: string; name?: string; value?: unknown; data?: { value?: unknown } };
        const value = item.dataType === 'edge' ? item.data?.value : item.value;
        return `${item.name ?? '外联实体'}<br/>关联强度：${value ?? '-'}`;
      },
    },
    series: [{
      name: '外联实体关系',
      type: 'graph',
      layout: 'none',
      roam: true,
      draggable: false,
      symbol: 'circle',
      edgeSymbol: ['none', 'arrow'],
      edgeSymbolSize: [0, 7],
      label: {
        show: true,
        position: 'right',
        color: '#d7f1ff',
        fontSize: 10,
        overflow: 'truncate',
        width: 74,
        textBorderColor: '#03111c',
        textBorderWidth: 3,
      },
      lineStyle: {
        color: 'source',
        curveness: 0.16,
        opacity: 0.48,
        width: 1.8,
      },
      emphasis: { focus: 'adjacency' },
      data: nodes.map((node) => ({
        name: node.name,
        x: node.x,
        y: node.y,
        value: node.value ?? 1,
        symbolSize: node.selected ? 27 : Math.max(15, Math.min(23, 13 + (node.value ?? 1) / 2)),
        itemStyle: {
          color: worldLevelColor(node.level, 'egress'),
          borderColor: node.selected ? '#f4fbff' : 'rgba(234, 247, 255, 0.68)',
          borderWidth: node.selected ? 3 : 1,
          shadowBlur: node.selected ? 20 : 10,
          shadowColor: worldLevelColor(node.level, 'egress'),
        },
      })),
      links: links.map((link) => ({
        source: link.source,
        target: link.target,
        value: link.value,
        lineStyle: link.selected ? { opacity: 1, width: 3.2 } : undefined,
      })),
    } as GraphSeriesOption],
  };

  return (
    <EChartsReactCore
      aria-label={ariaLabel}
      echarts={echarts}
      style={{ height: '100%', width: '100%' }}
      option={option}
      notMerge
      lazyUpdate
      onEvents={onNodeClick ? {
        click: (params: { seriesType?: string; dataType?: string; name?: string }) => {
          if (params.seriesType === 'graph' && params.dataType === 'node' && params.name) onNodeClick(params.name);
        },
      } : undefined}
    />
  );
}

const topicTopologyColors: Record<TopicTopologyNode['tone'], string> = {
  asset: '#18a8ff',
  probe: '#7fd4ff',
  risk: '#ff5b3d',
  protocol: '#b685ff',
  proxy: '#ffb020',
  destination: '#65d86e',
};

const topicTopologyLinkColors: Record<NonNullable<TopicTopologyLink['tone']>, string> = {
  info: 'rgba(55, 184, 255, 0.72)',
  risk: 'rgba(255, 91, 61, 0.72)',
  ok: 'rgba(116, 220, 100, 0.7)',
  warn: 'rgba(255, 176, 32, 0.72)',
  purple: 'rgba(196, 128, 255, 0.68)',
};

export function TopicTopologyGraph({
  ariaLabel,
  nodes,
  links,
  onNodeClick,
  showNodes = true,
  showNodeLabels = true,
}: {
  ariaLabel: string;
  nodes: TopicTopologyNode[];
  links: TopicTopologyLink[];
  onNodeClick?: (id: string) => void;
  showNodes?: boolean;
  showNodeLabels?: boolean;
}) {
  return (
    <svg className="taf-api-topology-svg" viewBox="0 0 100 100" preserveAspectRatio="none" role="img" aria-label={ariaLabel}>
      <defs>
        <filter id="taf-api-topology-glow" x="-50%" y="-50%" width="200%" height="200%"><feGaussianBlur stdDeviation="0.8" result="blur" /><feMerge><feMergeNode in="blur" /><feMergeNode in="SourceGraphic" /></feMerge></filter>
        <marker id="taf-api-topology-arrow" markerWidth="4" markerHeight="4" refX="3" refY="2" orient="auto"><path d="M 0 0 L 4 2 L 0 4 z" fill="rgba(127, 212, 255, 0.76)" /></marker>
      </defs>
      <g className="taf-api-topology-svg__links">
        {links.map((link) => {
          const source = nodes.find((node) => node.id === link.source);
          const target = nodes.find((node) => node.id === link.target);
          if (!source || !target) return null;
          const sourcePoint = topologySvgPoint(source);
          const targetPoint = topologySvgPoint(target);
          return <path key={`${link.source}-${link.target}`} d={`M ${sourcePoint.x} ${sourcePoint.y} L ${targetPoint.x} ${targetPoint.y}`} stroke={topicTopologyLinkColors[link.tone ?? 'info']} markerEnd={showNodes ? 'url(#taf-api-topology-arrow)' : undefined} />;
        })}
      </g>
      {showNodes && nodes.map((node) => {
        const point = topologySvgPoint(node);
        const width = Math.min(16, Math.max(8, (node.size?.[0] ?? 110) / 10));
        const height = Math.min(7, Math.max(3.5, (node.size?.[1] ?? 40) / 10));
        const color = topicTopologyColors[node.tone];
        const nodeProps = onNodeClick ? { role: 'button', tabIndex: 0, onClick: () => onNodeClick(node.id), onKeyDown: (event: KeyboardEvent<SVGGElement>) => { if (event.key === 'Enter' || event.key === ' ') onNodeClick(node.id); } } : {};
        return (
          <g key={node.id} className={`taf-api-topology-svg__node is-${node.tone} ${node.selected ? 'is-selected' : ''}`} {...nodeProps}>
            {showNodeLabels ? <><rect x={point.x - width / 2} y={point.y - height / 2} width={width} height={height} rx="1.2" fill="rgba(3, 17, 28, 0.9)" stroke={color} /><text x={point.x} y={point.y - 0.35} textAnchor="middle" className="taf-api-topology-svg__title">{node.label}</text><text x={point.x} y={point.y + 1.45} textAnchor="middle" className="taf-api-topology-svg__detail">{node.detail}</text></> : <><circle cx={point.x} cy={point.y} r="2" fill={color} opacity="0.18" /><circle cx={point.x} cy={point.y} r="0.72" fill={color} filter="url(#taf-api-topology-glow)" /><title>{`${node.label} ${node.detail}`}</title></>}
          </g>
        );
      })}
    </svg>
  );
}

const topologySvgPoint = (node: TopicTopologyNode) => ({
  x: Math.max(2, Math.min(98, node.x)),
  y: Math.max(2, Math.min(98, node.y)),
});

export function ExfilBarChart({
  items,
  ariaLabel,
}: {
  items: ExfilBarItem[];
  ariaLabel: string;
}) {
  const maxValue = Math.max(1, ...items.map((item) => item.value));
  const option: ChartOption = {
    backgroundColor: 'transparent',
    animation: false,
    tooltip: {
      trigger: 'axis',
      confine: true,
      backgroundColor: 'rgba(3, 17, 28, 0.94)',
      borderColor: 'rgba(127, 212, 255, 0.26)',
      textStyle: { color: '#eaf7ff', fontSize: 11 },
    },
    grid: { left: 70, right: 24, top: 4, bottom: 4 },
    xAxis: { type: 'value', min: 0, max: Math.ceil(maxValue * 1.12), show: false },
    yAxis: {
      type: 'category',
      inverse: true,
      data: items.map((item) => item.label),
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: { color: '#ccecff', fontSize: 10, width: 66, overflow: 'truncate' },
    },
    series: [
      {
        name: '命中',
        type: 'bar',
        data: items.map((item, index) => ({
          value: item.value,
          itemStyle: { color: index < 2 ? '#18a8ff' : index < 4 ? '#65d86e' : '#ffb020' },
        })),
        barWidth: 7,
        label: { show: true, position: 'right', color: '#d7f1ff', fontSize: 10 },
      },
    ],
  };

  return (
    <EChartsReactCore
      aria-label={ariaLabel}
      echarts={echarts}
      style={{ height: '100%', width: '100%' }}
      option={option}
      notMerge
      lazyUpdate
    />
  );
}

export function SparklineChart({
  trend,
  tone = 'ok',
}: {
  trend: number[];
  tone?: 'ok' | 'warn' | 'risk' | 'info';
}) {
  const color =
    tone === 'warn' ? '#ffb020' : tone === 'risk' ? '#ff4d4f' : tone === 'info' ? '#18a8ff' : '#36d66b';
  const source = trend.length ? trend : [42, 46, 43, 49, 45, 51, 48, 52];
  const min = Math.min(...source);
  const max = Math.max(...source);
  const range = Math.max(1, max - min);
  const normalized = source.map((value, index) => {
    const base = 44 + (((value - min) / range) - 0.5) * 8;
    const jitter = ((index * 7) % 5 - 2) * 0.8;
    return Math.max(36, Math.min(54, base + jitter));
  });
  const sparkData = normalized.flatMap((value, index) => {
    const next = normalized[index + 1];
    if (next === undefined) return [Math.round(value)];
    return [0, 1, 2].map((step) => {
      const t = step / 3;
      const baseline = value * (1 - t) + next * t;
      const wobble = ((index * 3 + step * 5) % 7 - 3) * 1.15;
      return Math.round(Math.max(36, Math.min(54, baseline + wobble)));
    });
  });
  const option: ChartOption = {
    backgroundColor: 'transparent',
    animation: false,
    tooltip: { show: false },
    grid: { left: 2, right: 2, top: 1, bottom: 1, containLabel: false },
    xAxis: { type: 'category', show: false, boundaryGap: false, data: sparkData.map((_, index) => `${index}`) },
    yAxis: { type: 'value', show: false, min: 30, max: 60 },
    series: [
      {
        name: '趋势',
        type: 'line',
        smooth: false,
        symbol: 'none',
        data: sparkData,
        lineStyle: { color, width: 1.35, shadowBlur: 5, shadowColor: `${color}88` },
        emphasis: { disabled: true },
      },
    ],
  };

  return (
    <EChartsReactCore
      className="taf-pipeline__chart"
      echarts={echarts}
      style={{ height: 16, width: '100%' }}
      option={option}
      notMerge
      lazyUpdate
    />
  );
}

export function AbnormalImpactPieChart({
  items,
  total,
  ariaLabel = '异常链路影响资产分布',
}: {
  items: AbnormalImpactItem[];
  total: number;
  ariaLabel?: string;
}) {
  const source = items.length
    ? items
    : [
        { name: '实验区 - 核心区', value: 432, level: 'high' as const },
        { name: '宿舍区 - 核心区', value: 311, level: 'medium' as const },
        { name: '办公区 - 核心区', value: 207, level: 'medium' as const },
      ];
  const palette = source.map((item, index) =>
    item.level === 'high' ? '#ff4d4f' : item.level === 'medium' ? '#ffb020' : index === 0 ? '#42a5f5' : '#36d66b',
  );
  const option: ChartOption = {
    backgroundColor: 'transparent',
    animationDuration: 450,
    title: {
      text: total.toLocaleString('zh-CN'),
      subtext: '异常影响资产',
      left: 'center',
      top: '38%',
      textStyle: { color: '#eaf7ff', fontSize: 24, fontWeight: 700 },
      subtextStyle: { color: '#9db9c9', fontSize: 12, fontWeight: 500 },
      itemGap: 4,
    },
    tooltip: {
      trigger: 'item',
      formatter: '{b}<br/>影响资产 {c} ({d}%)',
      backgroundColor: 'rgba(3,17,28,0.92)',
      borderColor: 'rgba(56,151,201,0.32)',
      textStyle: { color: '#eaf7ff', fontSize: 11 },
    },
    series: [
      {
        name: '异常影响资产',
        type: 'pie',
        radius: ['48%', '78%'],
        center: ['50%', '50%'],
        avoidLabelOverlap: true,
        minAngle: 8,
        label: { show: false },
        labelLine: { show: false },
        itemStyle: {
          borderColor: '#061827',
          borderWidth: 1.5,
        },
        data: source.map((item, index) => ({
          name: item.name,
          value: item.value,
          itemStyle: { color: palette[index] },
        })),
      },
    ],
  };

  return (
    <EChartsReactCore
      aria-label={ariaLabel}
      className="taf-impact-donut-chart"
      echarts={echarts}
      style={{ height: '100%', width: '100%' }}
      option={option}
      notMerge
      lazyUpdate
    />
  );
}

const evidenceRingColor = (level?: EvidenceRingItem['level']) => {
  if (level === 'high') return '#ff4d4f';
  if (level === 'medium') return '#ffb020';
  return '#49d86f';
};

export function EvidenceClosureRingChart({
  item,
  masked = false,
  ariaLabel,
}: {
  item: EvidenceRingItem;
  masked?: boolean;
  ariaLabel?: string;
}) {
  const value = Math.max(0, Math.min(100, Number.isFinite(item.value) ? item.value : 0));
  const option: ChartOption = {
    backgroundColor: 'transparent',
    animationDuration: 420,
    title: {
      text: masked ? '脱敏' : `${value.toFixed(1).replace(/\.0$/, '')}%`,
      left: 'center',
      top: '38%',
      textStyle: { color: '#eaf7ff', fontSize: 20, fontWeight: 800 },
    },
    tooltip: {
      trigger: 'item',
      formatter: `${item.label}<br/>闭环率 ${value.toFixed(1)}%`,
      backgroundColor: 'rgba(3,17,28,0.92)',
      borderColor: 'rgba(56,151,201,0.32)',
      textStyle: { color: '#eaf7ff', fontSize: 11 },
    },
    series: [
      {
        name: item.label,
        type: 'pie',
        radius: ['72%', '94%'],
        center: ['50%', '50%'],
        silent: true,
        clockwise: true,
        startAngle: 90,
        label: { show: false },
        labelLine: { show: false },
        data: [
          {
            name: item.label,
            value,
            itemStyle: { color: evidenceRingColor(item.level), borderColor: '#061827', borderWidth: 1 },
          },
          {
            name: '剩余',
            value: Math.max(0.001, 100 - value),
            itemStyle: { color: 'rgba(56,151,201,0.2)', borderColor: '#061827', borderWidth: 1 },
          },
        ],
      },
    ],
  };

  return (
    <EChartsReactCore
      aria-label={ariaLabel ?? `${item.label}动态图`}
      className="taf-evidence-ring__echart"
      echarts={echarts}
      style={{ height: '100%', width: '100%' }}
      option={option}
      notMerge
      lazyUpdate
    />
  );
}

const dashboardLevelColor = (level?: DashboardStageChartItem['level']) => {
  if (level === 'high') return '#ff4d4f';
  if (level === 'medium') return '#ffb020';
  if (level === 'info') return '#3fb7ff';
  return '#49d86f';
};

export function DashboardStageBasketChart({
  items,
  ariaLabel = '告警处置阶段工作篮动态图',
}: {
  items: DashboardStageChartItem[];
  ariaLabel?: string;
}) {
  const maxValue = Math.max(1, ...items.map((item) => item.value));
  const option: ChartOption = {
    backgroundColor: 'transparent',
    animationDuration: 420,
    legend: {
      top: 2,
      right: 8,
      itemWidth: 10,
      itemHeight: 8,
      textStyle: { color: '#9db9c9', fontSize: 11 },
      data: ['阶段数量', 'SLA'],
    },
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'shadow' },
      formatter: (params: unknown) => {
        const first = Array.isArray(params) ? params[0] as { dataIndex?: number; value?: number } : null;
        const item = first?.dataIndex === undefined ? null : items[first.dataIndex];
        return item
          ? `${item.label}<br/>当前数量 ${item.value}<br/>${item.footnote ?? ''}<br/>压力 ${Math.round(item.pressurePercent ?? 0)}%<br/>动作 ${item.action ?? '-'}`
          : '';
      },
      backgroundColor: 'rgba(3,17,28,0.94)',
      borderColor: 'rgba(56,151,201,0.32)',
      textStyle: { color: '#eaf7ff', fontSize: 11 },
    },
    grid: { left: 16, right: 24, top: 34, bottom: 74, containLabel: false },
    xAxis: {
      type: 'category',
      data: items.map((item) => item.label),
      axisTick: { show: false },
      axisLine: { lineStyle: { color: 'rgba(56,151,201,0.34)' } },
      axisLabel: {
        color: '#9db9c9',
        interval: 0,
        fontSize: 11,
        lineHeight: 14,
        width: 88,
        overflow: 'truncate',
        formatter: (value: string, index: number) => {
          const item = items[index];
          return item?.action ? `${value}\n${item.action}` : value;
        },
      },
    },
    yAxis: [
      {
        type: 'value',
        show: false,
        max: Math.max(5, Math.ceil(maxValue * 1.35)),
        splitLine: { show: false },
      },
      {
        type: 'value',
        show: false,
        min: 0,
        max: 100,
        splitLine: { show: false },
      },
    ],
    series: [
      {
        name: '压力',
        type: 'bar',
        yAxisIndex: 1,
        data: items.map((item) => ({
          value: Math.max(6, Math.min(100, item.pressurePercent ?? 0)),
          itemStyle: {
            color: 'rgba(56,151,201,0.08)',
            borderColor: 'rgba(56,151,201,0.16)',
            borderWidth: 1,
            borderRadius: [4, 4, 0, 0],
          },
        })),
        barGap: '-100%',
        barMaxWidth: 46,
        barMinHeight: 6,
        silent: true,
        tooltip: { show: false },
      },
      {
        name: '阶段数量',
        type: 'bar',
        yAxisIndex: 0,
        data: items.map((item) => ({
          value: item.value,
          itemStyle: {
            color: dashboardLevelColor(item.level),
            borderRadius: [4, 4, 0, 0],
            shadowBlur: 10,
            shadowColor: dashboardLevelColor(item.level),
          },
        })),
        barMaxWidth: 22,
        barMinHeight: 4,
        label: {
          show: true,
          position: 'top',
          formatter: '{c} 项',
          color: '#eaf7ff',
          fontSize: 16,
          fontWeight: 800,
        },
      },
      {
        name: 'SLA',
        type: 'line',
        yAxisIndex: 1,
        data: items.map((item) => Math.max(0, Math.min(100, item.slaPercent ?? 0))),
        symbol: 'circle',
        symbolSize: 7,
        smooth: true,
        lineStyle: { color: '#7fd4ff', width: 2 },
        itemStyle: { color: '#7fd4ff', borderColor: '#082033', borderWidth: 1 },
        label: {
          show: true,
          position: 'bottom',
          formatter: 'SLA {c}%',
          color: '#9db9c9',
          fontSize: 10,
          distance: 10,
        },
      },
    ],
  };

  return (
    <EChartsReactCore
      aria-label={ariaLabel}
      className="taf-dashboard-stage__echart"
      echarts={echarts}
      style={{ height: '100%', width: '100%' }}
      option={option}
      notMerge
      lazyUpdate
    />
  );
}

export function DashboardStageRateCardChart({
  label,
  values,
  level,
  ariaLabel,
}: {
  label: string;
  values: number[];
  level?: DashboardStageChartItem['level'];
  ariaLabel?: string;
}) {
  const days = values.slice(-10);
  const option: ChartOption = {
    backgroundColor: 'transparent',
    animationDuration: 420,
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'shadow' },
      formatter: (params: unknown) => {
        const first = Array.isArray(params) ? params[0] as { dataIndex?: number; value?: number } : null;
        return first ? `${label}<br/>前 ${10 - Number(first.dataIndex ?? 0)} 日达成率 ${first.value}%` : '';
      },
      backgroundColor: 'rgba(3,17,28,0.94)',
      borderColor: 'rgba(56,151,201,0.32)',
      textStyle: { color: '#eaf7ff', fontSize: 11 },
    },
    grid: { left: 2, right: 2, top: 4, bottom: 2 },
    xAxis: {
      type: 'category',
      data: days.map((_, index) => `D-${days.length - index}`),
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: { show: false },
    },
    yAxis: {
      type: 'value',
      min: 0,
      max: 100,
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: { show: false },
      splitLine: { show: false },
    },
    series: [
      {
        name: '达成率',
        type: 'bar',
        data: days.map((value) => Math.max(0, Math.min(100, Math.round(value)))),
        barWidth: 7,
        itemStyle: {
          color: dashboardLevelColor(level),
          borderRadius: [2, 2, 0, 0],
          shadowBlur: 8,
          shadowColor: dashboardLevelColor(level),
        },
      },
    ],
  };

  return (
    <EChartsReactCore
      aria-label={ariaLabel ?? `${label}前十日达成率动态图`}
      className="taf-dashboard-stage-rate__echart"
      echarts={echarts}
      style={{ height: '100%', width: '100%' }}
      option={option}
      notMerge
      lazyUpdate
    />
  );
}

export function DashboardStageSlaBarsChart({
  item,
  ariaLabel,
}: {
  item: DashboardStageSlaBarsItem;
  ariaLabel?: string;
}) {
  const color = dashboardLevelColor(item.level);
  const values = item.values.slice(-10).map((value) => Math.max(0, Math.min(100, value)));
  const option: ChartOption = {
    backgroundColor: 'transparent',
    animationDuration: 420,
    tooltip: {
      trigger: 'axis',
      formatter: (params: unknown) => {
        const first = Array.isArray(params) ? params[0] as { dataIndex?: number; value?: number } : null;
        return first ? `前 ${10 - Number(first.dataIndex)} 日<br/>达成率 ${first.value}%` : '';
      },
      backgroundColor: 'rgba(3,17,28,0.94)',
      borderColor: 'rgba(56,151,201,0.32)',
      textStyle: { color: '#eaf7ff', fontSize: 11 },
    },
    grid: { left: 2, right: 2, top: 4, bottom: 4 },
    xAxis: {
      type: 'category',
      data: values.map((_, index) => `D-${values.length - index}`),
      show: false,
    },
    yAxis: {
      type: 'value',
      min: 0,
      max: 100,
      show: false,
      splitLine: { show: false },
    },
    series: [
      {
        name: '前十日达成率',
        type: 'bar',
        data: values.map((value) => ({
          value,
          itemStyle: {
            color,
            borderRadius: [2, 2, 0, 0],
            shadowBlur: 7,
            shadowColor: color,
          },
        })),
        barWidth: 6,
        barMinHeight: 4,
      },
    ],
  };

  return (
    <EChartsReactCore
      aria-label={ariaLabel ?? `${item.label}前十日达成率`}
      className="taf-stage-bars__echart"
      echarts={echarts}
      style={{ height: '100%', width: '100%' }}
      option={option}
      notMerge
      lazyUpdate
    />
  );
}

export function DashboardKpiSparklineChart({
  item,
  ariaLabel,
}: {
  item: DashboardKpiSparklineItem;
  ariaLabel?: string;
}) {
  const color = dashboardLevelColor(item.level);
  const values = item.values.slice(-26).map((value) => Math.max(0, Math.round(value)));
  const option: ChartOption = {
    backgroundColor: 'transparent',
    animationDuration: 360,
    tooltip: {
      trigger: 'axis',
      formatter: (params: unknown) => {
        const first = Array.isArray(params) ? params[0] as { value?: number } : null;
        return first ? `${item.label}<br/>趋势值 ${first.value}` : '';
      },
      backgroundColor: 'rgba(3,17,28,0.94)',
      borderColor: 'rgba(56,151,201,0.32)',
      textStyle: { color: '#eaf7ff', fontSize: 11 },
    },
    grid: { left: 0, right: 0, top: 4, bottom: 2 },
    xAxis: {
      type: 'category',
      data: values.map((_, index) => `${index + 1}`),
      show: false,
    },
    yAxis: {
      type: 'value',
      show: false,
      min: Math.max(0, Math.min(...values) - 4),
      max: Math.max(...values, 10) + 4,
      splitLine: { show: false },
    },
    series: [
      {
        name: item.label,
        type: 'line',
        data: values,
        smooth: false,
        symbol: 'none',
        lineStyle: { color, width: 2 },
        areaStyle: { color: 'rgba(56,151,201,0.04)' },
      },
    ],
  };

  return (
    <EChartsReactCore
      aria-label={ariaLabel ?? `${item.label}实时趋势`}
      className="taf-dashboard-kpi-spark__echart"
      echarts={echarts}
      style={{ height: '100%', width: '100%' }}
      option={option}
      notMerge
      lazyUpdate
    />
  );
}

export function DashboardTopTalkersChart({
  items,
  ariaLabel = 'Top Talkers 风险贡献动态图',
}: {
  items: DashboardTalkerChartItem[];
  ariaLabel?: string;
}) {
  const sorted = [...items].sort((left, right) => left.value - right.value);
  const option: ChartOption = {
    backgroundColor: 'transparent',
    animationDuration: 420,
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'shadow' },
      formatter: (params: unknown) => {
        const first = Array.isArray(params) ? params[0] as { name?: string; value?: number } : null;
        return first ? `${first.name}<br/>风险贡献度 ${first.value}%` : '';
      },
      backgroundColor: 'rgba(3,17,28,0.94)',
      borderColor: 'rgba(56,151,201,0.32)',
      textStyle: { color: '#eaf7ff', fontSize: 11 },
    },
    grid: { left: 76, right: 42, top: 12, bottom: 10 },
    xAxis: {
      type: 'value',
      max: 100,
      show: false,
      splitLine: { show: false },
    },
    yAxis: {
      type: 'category',
      data: sorted.map((item) => item.label),
      axisTick: { show: false },
      axisLine: { show: false },
      axisLabel: { color: '#9db9c9', fontSize: 12 },
    },
    series: [
      {
        name: '风险贡献度',
        type: 'bar',
        data: sorted.map((item) => ({
          value: item.value,
          itemStyle: {
            color: {
              type: 'linear',
              x: 0,
              y: 0,
              x2: 1,
              y2: 0,
              colorStops: [
                { offset: 0, color: '#ff4d4f' },
                { offset: 1, color: '#ffb020' },
              ],
            },
            borderRadius: [8, 8, 8, 8],
          },
        })),
        barWidth: 8,
        showBackground: true,
        backgroundStyle: { color: 'rgba(56,151,201,0.12)', borderRadius: 8 },
        label: {
          show: true,
          position: 'right',
          formatter: '{c}%',
          color: '#eaf7ff',
          fontSize: 12,
        },
      },
    ],
  };

  return (
    <EChartsReactCore
      aria-label={ariaLabel}
      className="taf-dashboard-talkers__echart"
      echarts={echarts}
      style={{ height: '100%', width: '100%' }}
      option={option}
      notMerge
      lazyUpdate
    />
  );
}

export function TrendChart({ title = '近 24 小时趋势' }: { title?: string }) {
  const option: ChartOption = {
    backgroundColor: 'transparent',
    title: { text: title, textStyle: { color: '#9db9c9', fontSize: 12 }, top: 4, left: 4 },
    tooltip: { trigger: 'axis' },
    grid: { left: 28, right: 12, top: 42, bottom: 22 },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: ['00', '03', '06', '09', '12', '15', '18', '21'],
      axisLine: { lineStyle: { color: '#24485f' } },
      axisLabel: { color: '#7998aa' },
    },
    yAxis: {
      type: 'value',
      splitLine: { lineStyle: { color: 'rgba(56,151,201,0.14)' } },
      axisLabel: { color: '#7998aa' },
    },
    series: [
      {
        name: '风险',
        type: 'line',
        smooth: true,
        data: [32, 45, 38, 67, 72, 61, 83, 76],
        symbolSize: 6,
        lineStyle: { color: '#18a8ff', width: 2 },
        areaStyle: { color: 'rgba(24,168,255,0.16)' },
      },
      {
        name: '闭环',
        type: 'line',
        smooth: true,
        data: [21, 28, 31, 44, 53, 62, 69, 74],
        symbolSize: 6,
        lineStyle: { color: '#36d66b', width: 2 },
        areaStyle: { color: 'rgba(54,214,107,0.12)' },
      },
    ],
  };

  return (
    <EChartsReactCore
      echarts={echarts}
      style={{ height: 210, width: '100%' }}
      option={option}
    />
  );
}

export function RingChart({
  value = 68,
  height = 160,
  className,
  ariaLabel = '环形仪表图',
  suffix = '%',
}: {
  value?: number;
  height?: number | string;
  className?: string;
  ariaLabel?: string;
  suffix?: string;
}) {
  const option: ChartOption = {
    backgroundColor: 'transparent',
    series: [
      {
        type: 'gauge',
        startAngle: 210,
        endAngle: -30,
        min: 0,
        max: 100,
        progress: { show: true, width: 12, itemStyle: { color: '#36d66b' } },
        axisLine: { lineStyle: { width: 12, color: [[1, 'rgba(56,151,201,0.18)']] } },
        pointer: { show: false },
        axisTick: { show: false },
        splitLine: { show: false },
        axisLabel: { show: false },
        detail: { valueAnimation: true, color: '#eaf7ff', fontSize: 24, formatter: `{value}${suffix}` },
        data: [{ value }],
      },
    ],
  };

  return (
    <EChartsReactCore
      aria-label={ariaLabel}
      className={className}
      echarts={echarts}
      style={{ height, width: '100%' }}
      option={option}
    />
  );
}

export function RuleHitComparisonChart({ ariaLabel = '规则生效前后命中差异' }: { ariaLabel?: string }) {
  const option: ChartOption = {
    backgroundColor: 'transparent',
    animationDuration: 320,
    tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
    legend: { right: 2, top: 0, textStyle: { color: '#9fb9ca', fontSize: 7 }, itemWidth: 10, itemHeight: 4, itemGap: 6 },
    grid: { left: 32, right: 8, top: 18, bottom: 2 },
    xAxis: { type: 'value', show: false, max: 1800 },
    yAxis: {
      type: 'category',
      data: ['命中率', '命中数'],
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: { color: '#b9ceda', fontSize: 7 },
    },
    series: [
      {
        name: '规则生效前',
        type: 'bar',
        data: [1102, 1102],
        barWidth: 6,
        itemStyle: { color: '#2493ff', borderRadius: [0, 5, 5, 0] },
        label: { show: true, position: 'insideRight', color: '#d8efff', fontSize: 7, formatter: (params: { dataIndex?: number }) => params.dataIndex === 0 ? '55.1%' : '1,102' },
      },
      {
        name: '规则生效后',
        type: 'bar',
        data: [1568, 1568],
        barWidth: 6,
        itemStyle: { color: '#50c73f', borderRadius: [0, 5, 5, 0] },
        label: { show: true, position: 'insideRight', color: '#e6ffe1', fontSize: 7, formatter: (params: { dataIndex?: number }) => params.dataIndex === 0 ? '78.5%' : '1,568' },
      },
    ],
  };
  return <EChartsReactCore aria-label={ariaLabel} echarts={echarts} style={{ width: '100%', height: '100%' }} option={option} notMerge lazyUpdate />;
}

export function RuleDependencyGraphChart({ ruleId, ariaLabel = '规则依赖引用图' }: { ruleId: string; ariaLabel?: string }) {
  const ruleLabel = `RULE\n${ruleId.replace(/_v(\d+)$/i, '\nv$1')}`;
  const label = (type: string, name: string, badge: string, position: 'top' | 'bottom' | 'left' | 'right', color: string) => ({
    show: true,
    position,
    formatter: `{type|${type}}\n{name|${name}}\n{badge|${badge}}`,
    rich: {
      type: { color, fontSize: 8, fontWeight: 700, lineHeight: 10 },
      name: { color: '#eaf7ff', fontSize: 8, fontWeight: 600, lineHeight: 10 },
      badge: { color, fontSize: 7, fontWeight: 700, lineHeight: 9, backgroundColor: 'rgba(3,17,28,0.88)', borderColor: color, borderWidth: 1, borderRadius: 2, padding: [1, 3] },
    },
  });
  const data = [
    { id: 'rule', name: ruleLabel, value: ruleId, x: 50, y: 50, symbol: 'roundRect', symbolSize: [68, 50], category: 0, itemStyle: { color: '#072b46', borderColor: '#18a8ff', borderWidth: 2.5, shadowBlur: 14, shadowColor: 'rgba(24,168,255,0.62)' }, label: { position: 'inside' as const, color: '#eaf7ff', fontSize: 8, fontWeight: 700, lineHeight: 10 } },
    { id: 'model', name: 'Model-XGB-17', value: '关联模型 Model-XGB-17 v2.3.1', x: 25, y: 27, symbolSize: 33, category: 1, label: label('关联模型', 'Model-XGB-17', 'v2.3.1', 'top', '#c084fc') },
    { id: 'whitelist', name: 'WL-VPN-003', value: '白名单 WL-VPN-003 生效中', x: 75, y: 27, symbolSize: 33, category: 2, label: label('白名单', 'WL-VPN-003', '生效中', 'top', '#59d75d') },
    { id: 'deploy', name: 'PROD-北区', value: '部署环境 PROD-北区 生产', x: 14, y: 50, symbolSize: 33, category: 3, label: label('部署环境', 'PROD-北区', '生产', 'left', '#31b6ff') },
    { id: 'source', name: 'detections.v1', value: '数据源 detections.v1 实时', x: 86, y: 50, symbolSize: 33, category: 4, label: label('数据源', 'detections.v1', '实时', 'right', '#ffb21c') },
    { id: 'fields', name: 'src_ip / dst_port', value: '关键字段 src_ip / dst_port 映射完整', x: 27, y: 73, symbolSize: 33, category: 5, label: label('关键字段', 'src_ip/dst_port', '映射完整', 'bottom', '#ff851b') },
    { id: 'alert', name: 'C2 Beacon', value: '告警类型 C2 Beacon 高危', x: 73, y: 73, symbolSize: 33, category: 6, label: label('告警类型', 'C2 Beacon', '高危', 'bottom', '#ff5454') },
  ];
  const option: ChartOption = {
    backgroundColor: 'transparent',
    animationDuration: 420,
    tooltip: { trigger: 'item', backgroundColor: 'rgba(3,17,28,0.96)', borderColor: 'rgba(56,151,201,0.4)', textStyle: { color: '#eaf7ff', fontSize: 11 } },
    series: [{
      type: 'graph',
      layout: 'none',
      left: 66,
      right: 66,
      top: 36,
      bottom: 36,
      roam: true,
      scaleLimit: { min: 0.85, max: 1.8 },
      data,
      links: data.slice(1).map((node, index) => ({
        source: 'rule',
        target: node.id,
        lineStyle: { color: ['#a866f2', '#59d75d', '#31b6ff', '#ffb21c', '#ff851b', '#ff5454'][index] },
      })),
      categories: [
        { name: '规则', itemStyle: { color: '#072b46', borderColor: '#18a8ff', borderWidth: 2 } },
        { name: '模型', itemStyle: { color: '#160d25', borderColor: '#a866f2', borderWidth: 2, shadowBlur: 8, shadowColor: 'rgba(168,102,242,0.45)' } },
        { name: '白名单', itemStyle: { color: '#0c2517', borderColor: '#59d75d', borderWidth: 2, shadowBlur: 8, shadowColor: 'rgba(89,215,93,0.4)' } },
        { name: '部署', itemStyle: { color: '#092136', borderColor: '#31b6ff', borderWidth: 2, shadowBlur: 8, shadowColor: 'rgba(49,182,255,0.4)' } },
        { name: '数据源', itemStyle: { color: '#2a2108', borderColor: '#ffb21c', borderWidth: 2, shadowBlur: 8, shadowColor: 'rgba(255,178,28,0.4)' } },
        { name: '字段', itemStyle: { color: '#2a1608', borderColor: '#ff851b', borderWidth: 2, shadowBlur: 8, shadowColor: 'rgba(255,133,27,0.4)' } },
        { name: '告警', itemStyle: { color: '#2b0d12', borderColor: '#ff5454', borderWidth: 2, shadowBlur: 8, shadowColor: 'rgba(255,84,84,0.4)' } },
      ],
      symbol: 'circle',
      edgeSymbol: ['none', 'arrow'],
      edgeSymbolSize: [0, 6],
      label: { show: true, color: '#d7e9f4', fontSize: 8, lineHeight: 10, distance: 6, overflow: 'truncate' },
      lineStyle: { width: 1.8, type: 'dashed', opacity: 0.88, curveness: 0.04 },
      emphasis: { focus: 'adjacency', lineStyle: { width: 3, opacity: 1 } },
    }],
  };
  return <EChartsReactCore aria-label={ariaLabel} echarts={echarts} style={{ width: '100%', height: '100%' }} option={option} notMerge lazyUpdate />;
}
