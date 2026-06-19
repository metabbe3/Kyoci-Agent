import React, { useState, useEffect } from 'react';
import { Calculator } from './calculator';

interface DisplayProps {
  currentValue: string;
  previousValue: string | null;
  operator: string | null;
  error: string | null;
}

const Display: React.FC<DisplayProps> = ({ currentValue, previousValue, operator, error }) => {
  const [showPrevious, setShowPrevious] = useState(false);

  useEffect(() => {
    if (previousValue && operator) {
      setShowPrevious(true);
    } else {
      setShowPrevious(false);
    }
  }, [previousValue, operator]);

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (['0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '.', '+', '-', '*', '/', '(', ')', 'C', '⌫', '=', '÷', '×'].includes(e.key)) {
      e.preventDefault();
    }
  };

  return (
    <div className="display-container">
      <div className="error-message">{error}</div>
      <div className="previous-display">
        {showPrevious && (
          <span className="previous-value">
            {previousValue} {operator}
          </span>
        )}
      </div>
      <div className="current-display">
        <span className="current-value">{currentValue}</span>
      </div>
    </div>
  );
};

interface ButtonProps {
  label: string;
  onClick: () => void;
  className?: string;
  variant?: 'number' | 'operator' | 'action';
}

const Button: React.FC<ButtonProps> = ({ label, onClick, className = '', variant = 'number' }) => {
  const [pressed, setPressed] = useState(false);

  const handleMouseDown = () => {
    setPressed(true);
    onClick();
  };

  const handleMouseUp = () => {
    setPressed(false);
  };

  const handleMouseLeave = () => {
    setPressed(false);
  };

  const getButtonStyles = () => {
    const baseStyles = 'calculator-button';
    const variantStyles = {
      number: 'button-number',
      operator: 'button-operator',
      action: 'button-action',
    };
    const pressStyles = pressed ? 'button-pressed' : '';
    return `${baseStyles} ${variantStyles[variant]} ${pressStyles} ${className}`;
  };

  return (
    <button
      className={getButtonStyles()}
      onMouseDown={handleMouseDown}
      onMouseUp={handleMouseUp}
      onMouseLeave={handleMouseLeave}
      onTouchStart={handleMouseDown}
      onTouchEnd={handleMouseUp}
      onTouchCancel={handleMouseLeave}
    >
      {label}
    </button>
  );
};

const CalculatorApp: React.FC = () => {
  const calculator = new Calculator();
  const [display, setDisplay] = useState<DisplayProps>({
    currentValue: '0',
    previousValue: null,
    operator: null,
    error: null,
  });

  const handleInput = (value: string) => {
    calculator.input(value);
    const state = calculator.getState();
    setDisplay({
      currentValue: state.currentValue,
      previousValue: state.previousValue,
      operator: state.operator,
      error: state.error,
    });
  };

  const handleClear = () => {
    calculator.clear();
    const state = calculator.getState();
    setDisplay({
      currentValue: state.currentValue,
      previousValue: state.previousValue,
      operator: state.operator,
      error: state.error,
    });
  };

  const handleDelete = () => {
    calculator.delete();
    const state = calculator.getState();
    setDisplay({
      currentValue: state.currentValue,
      previousValue: state.previousValue,
      operator: state.operator,
      error: state.error,
    });
  };

  const handleOperator = (op: string) => {
    calculator.chooseOperator(op);
    const state = calculator.getState();
    setDisplay({
      currentValue: state.currentValue,
      previousValue: state.previousValue,
      operator: state.operator,
      error: state.error,
    });
  };

  const handleEquals = () => {
    calculator.equals();
    const state = calculator.getState();
    setDisplay({
      currentValue: state.currentValue,
      previousValue: state.previousValue,
      operator: state.operator,
      error: state.error,
    });
  };

  const handleHistory = () => {
    const history = calculator.getHistory();
    if (history.length > 0) {
      const lastCalculation = history[history.length - 1];
      alert(`History:\n${lastCalculation.expression} = ${lastCalculation.result}`);
    }
  };

  const handleClearHistory = () => {
    calculator.clearHistory();
    const state = calculator.getState();
    setDisplay({
      currentValue: state.currentValue,
      previousValue: state.previousValue,
      operator: state.operator,
      error: state.error,
    });
  };

  return (
    <div className="calculator-app">
      <div className="calculator-header">
        <h1>Calculator</h1>
      </div>
      
      <div className="calculator-body">
        <Display
          currentValue={display.currentValue}
          previousValue={display.previousValue}
          operator={display.operator}
          error={display.error}
        />
        
        <div className="calculator-buttons">
          <div className="button-row">
            <Button label="C" onClick={handleClear} variant="action" />
            <Button label="⌫" onClick={handleDelete} variant="action" />
            <Button label="÷" onClick={() => handleOperator('/')} variant="operator" />
            <Button label="×" onClick={() => handleOperator('*')} variant="operator" />
          </div>
          <div className="button-row">
            <Button label="7" onClick={() => handleInput('7')} variant="number" />
            <Button label="8" onClick={() => handleInput('8')} variant="number" />
            <Button label="9" onClick={() => handleInput('9')} variant="number" />
            <Button label="-" onClick={() => handleOperator('-')} variant="operator" />
          </div>
          <div className="button-row">
            <Button label="4" onClick={() => handleInput('4')} variant="number" />
            <Button label="5" onClick={() => handleInput('5')} variant="number" />
            <Button label="6" onClick={() => handleInput('6')} variant="number" />
            <Button label="+" onClick={() => handleOperator('+')} variant="operator" />
          </div>
          <div className="button-row">
            <Button label="1" onClick={() => handleInput('1')} variant="number" />
            <Button label="2" onClick={() => handleInput('2')} variant="number" />
            <Button label="3" onClick={() => handleInput('3')} variant="number" />
            <Button label="=" onClick={handleEquals} variant="action" />
          </div>
          <div className="button-row">
            <Button label="0" onClick={() => handleInput('0')} variant="number" />
            <Button label="." onClick={() => handleInput('.')} variant="number" />
            <Button label="(" onClick={() => handleInput('(')} variant="number" />
            <Button label=")" onClick={() => handleInput(')')} variant="number" />
          </div>
        </div>
      </div>
    </div>
  );
};

export default CalculatorApp;