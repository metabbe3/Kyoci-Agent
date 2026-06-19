import React, { useState } from 'react';

interface CalculatorState {
  display: string;
}

const initialState: CalculatorState = { display: '0' };

const App: React.FC = () => {
  const [state, setState] = useState<CalculatorState>(initialState);

  const handleDigitClick = (digit: string) => {
    setState(prevState => ({
      display: prevState.display === '0' ? digit : prevState.display + digit,
    }));
  };

  const handleClearClick = () => {
    setState(initialState);
  };

  return (
    <div className="calculator">
      <input type="text" value={state.display} readOnly />
      <button onClick={() => handleDigitClick('7')}>7</button>
      <button onClick={() => handleDigitClick('8')}>8</button>
      <button onClick={() => handleDigitClick('9')}>9</button>
      <button onClick={handleClearClick}>C</button>
      <button onClick={() => handleDigitClick('4')}>4</button>
      <button onClick={() => handleDigitClick('5')}>5</button>
      <button onClick={() => handleDigitClick('6')}>6</button>
      <button onClick={() => handleDigitClick('1')}>1</button>
      <button onClick={() => handleDigitClick('2')}>2</button>
      <button onClick={() => handleDigitClick('3')}>3</button>
      <button onClick={() => handleDigitClick('0')}>0</button>
      <button onClick={() => handleDigitClick('.')}>.</button>
      <button>=</button>
    </div>
  );
};

export default App;
