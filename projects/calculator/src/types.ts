export interface CalculatorState {
  display: string;
  currentInput: number | null;
  previousInput: number | null;
  operator: string | null;
}

export interface Button {
  label: string;
  value?: number | null;
  operator?: boolean;
  type?: 'primary' | 'secondary';
}
