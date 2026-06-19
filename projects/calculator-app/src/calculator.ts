export interface Calculation {
  id: string;
  expression: string;
  result: number;
  timestamp: Date;
}

export interface CalculatorState {
  currentValue: string;
  previousValue: string | null;
  operator: string | null;
  history: Calculation[];
  error: string | null;
}

export class Calculator {
  private state: CalculatorState;

  constructor() {
    this.state = {
      currentValue: '0',
      previousValue: null,
      operator: null,
      history: [],
      error: null,
    };
  }

  private generateId(): string {
    return Date.now().toString(36) + Math.random().toString(36).substr(2);
  }

  private validateInput(input: string): boolean {
    if (!input) return false;
    if (input === 'Infinity' || input === '-Infinity') return false;
    const num = parseFloat(input);
    return !isNaN(num) && isFinite(num);
  }

  private sanitizeInput(input: string): string {
    const validChars = ['0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '.', '+', '-', '*', '/', '(', ')'];
    return input.split('').filter(char => validChars.includes(char)).join('');
  }

  private validateExpression(expression: string): boolean {
    if (!expression) return false;
    if (expression === 'Infinity' || expression === '-Infinity') return false;
    
    const numRegex = /^-?\d+(\.\d+)?$/;
    const openParens = (expression.match(/\(/g) || []).length;
    const closeParens = (expression.match(/\)/g) || []).length;
    
    if (openParens !== closeParens) return false;
    
    const numbers = expression.match(/-?\d+(\.\d+)?/g) || [];
    if (numbers.length < 2) return false;
    
    return true;
  }

  private calculate(expression: string): number {
    const sanitized = this.sanitizeInput(expression);
    if (!this.validateExpression(sanitized)) {
      throw new Error('Invalid expression');
    }
    
    try {
      // Use Function constructor with strict validation
      const result = new Function('return ' + sanitized)();
      
      if (!this.validateInput(result.toString())) {
        throw new Error('Invalid result');
      }
      
      return result;
    } catch (error) {
      throw new Error('Invalid expression');
    }
  }

  private formatResult(result: number): string {
    const absResult = Math.abs(result);
    if (absResult === Math.floor(absResult)) {
      return result.toString();
    }
    const resultStr = result.toFixed(8);
    return resultStr.replace(/\.?0+$/, '');
  }

  public input(value: string): void {
    if (this.state.error) {
      this.reset();
      value = this.sanitizeInput(value);
    }
    
    if (this.state.operator && !this.state.previousValue) {
      this.calculate(this.state.previousValue + value);
    } else if (this.state.previousValue) {
      this.state.currentValue = value;
    } else {
      this.state.currentValue = value;
    }
  }

  public clear(): void {
    this.state = {
      currentValue: '0',
      previousValue: null,
      operator: null,
      history: [],
      error: null,
    };
  }

  public delete(): void {
    if (this.state.error) {
      this.reset();
      return;
    }
    
    if (this.state.currentValue.length > 1) {
      this.state.currentValue = this.state.currentValue.slice(0, -1);
    } else {
      this.state.currentValue = '0';
    }
  }

  public chooseOperator(operator: string): void {
    if (this.state.error) return;
    
    if (this.state.previousValue) {
      this.calculate(this.state.previousValue + this.state.currentValue);
      this.state.previousValue = null;
      this.state.operator = null;
    }
    
    this.state.operator = operator;
  }

  public equals(): void {
    if (this.state.error || !this.state.operator || !this.state.previousValue) return;
    
    try {
      const expression = this.state.previousValue + this.state.operator + this.state.currentValue;
      const result = this.calculate(expression);
      
      const calculation: Calculation = {
        id: this.generateId(),
        expression: expression,
        result: result,
        timestamp: new Date(),
      };
      
      this.state.history.push(calculation);
      this.state.currentValue = this.formatResult(result);
      this.state.previousValue = null;
      this.state.operator = null;
      this.state.error = null;
    } catch (error) {
      this.state.error = 'Invalid expression';
    }
  }

  public getHistory(): Calculation[] {
    return this.state.history;
  }

  public clearHistory(): void {
    this.state.history = [];
  }

  public getState(): CalculatorState {
    return { ...this.state };
  }

  public reset(): void {
    this.clear();
  }
}