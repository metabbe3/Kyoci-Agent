import React from 'react';
import ReactDOM from 'react-dom';
import App from './App';

const handleButtonClick = () => {
  console.log('Button clicked!');
};

ReactDOM.render(<App />, document.getElementById('root'));