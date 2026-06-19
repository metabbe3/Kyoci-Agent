/**
 * Calculator Logic Implementation (TypeScript/JavaScript)
 * Handles all calculation operations and UI updates.
 */

class Calculator {
    constructor(previousOperandTextElement, currentOperandTextElement) {
        this.currentOperandTextElement = currentOperandTextElement;
        this.previousOperandTextElement = previousOperandTextElement;
        this.currentOperand = '0';
        this.previousOperand = '';
        this.operation = undefined;
    }

    /**
     * Clears all operands and operations.
     */
    clear() {
        this.currentOperand = '0';
        this.previousOperand = '';
        this.operation = undefined;
    }

    /**
     * Resets the current operand to '0' or deletes the last character.
     */
    delete() {
        if (this.currentOperand === 'Error') return; // Prevent deletion if in error state

        // If the current operand is a single digit and we delete it, reset to '0'
        if (this.currentOperand.length === 1 && this.currentOperand !== '0') {
            this.currentOperand = '0';
        } else {
            // Otherwise, remove the last character
            this.currentOperand = this.currentOperand.toString().slice(0, -1);
            if (this.currentOperand === '') {
                this.currentOperand = '0';
            }
        }
    }

    /**
     * Appends a number or decimal point to the current operand.
     * @param {string} number - The input number string.
     */
    appendNumber(number) {
        // Prevent multiple decimal points in the current operand
        if (number === '.' && this.currentOperand.includes('.')) return;

        // If the current operand is '0' and we receive a number, replace it unless it's a decimal
        if (this.currentOperand === '0' && number !== '.') {
            this.currentOperand = number;
        } else {
            this.currentOperand = this.currentOperand + number;
        }
    }

    /**
     * Selects the operation (+, -, *, /) and prepares for calculation.
     * @param {string} operation - The mathematical operation symbol.
     */
    chooseOperation(operation) {
        if (this.currentOperand === '') return; // Don't proceed if current input is empty

        // If there was a previous operation pending, calculate it first (chaining operations)
        if (this.operation != null && this.currentOperand !== '') {
            this.calculate();
        }

        this.operation = operation;
        // Move current operand to previous operand display for the next calculation step
        this.previousOperand = this.currentOperand;
    }

    /**
     * Performs the calculation based on the stored operation.
     */
    calculate() {
        let currentOperand = parseFloat(this.currentOperand);
        let previousOperand = parseFloat(this.previousOperand);

        if (isNaN(currentOperand) || isNaN(previousOperand)) return; // Handle non-numeric inputs

        let computationResult = 0;
        const currentOp = this.operation;

        try {
            switch (currentOp) {
                case '+':
                    computationResult = previousOperand + currentOperand;
                    break;
                case '-':
                    computationResult = previousOperand - currentOperand;
                    break;
                case '*':
                    computationResult = previousOperand * currentOperand;
                    break;
                case '/':
                    if (currentOperand === 0) {
                        computationResult = 'Error'; // Division by zero handling
                    } else {
                        computationResult = previousOperand / currentOperand;
                    }
                    break;
                default:
                    return; // No operation selected
            }

            // Update the current operand with the result, formatted nicely
            this.currentOperand = computationResult.toString();
            if (typeof computationResult === 'number') {
                // Limit floating point errors and display cleanly
                this.currentOperand = parseFloat(computationResult.toFixed(10)).toString();
            }

        } catch (e) {
            this.currentOperand = 'Error'; // Catch unexpected calculation errors
        } finally {
            // Clear the operation after calculation is done
            this.operation = undefined;
            this.previousOperand = ''; // Clear previous operand after successful calculation step
        }
    }

    /**
     * Handles the equals action, triggering the calculation.
     */
    equals() {
        this.calculate();
    }

    /**
     * Displays the current operand on the screen.
     */
    getDisplayValues() {
        this.currentOperandTextElement.innerText = this.currentOperand;
        this.previousOperandTextElement.innerText = this.operation ? `${this.currentOperand} ${this.operation}` : '';
    }

    /**
     * Lifecycle method to update the UI elements.
     */
    updateDisplay() {
        this.getDisplayValues();
    }
}

// --- DOM Element Selection and Initialization ---

// Select all necessary elements from the HTML structure
const numberButtons = document.querySelectorAll('[data-number]');
const operatorButtons = document.querySelectorAll('[data-operation]');
const equalsButton = document.querySelector('[data-action="equals"]');
const deleteButton = document.querySelector('[data-action="backspace"]'); // Using backspace for DEL
const clearButton = document.querySelector('[data-action="clear"]');
const currentOperandTextElement = document.getElementById('current-operand');
const previousOperandTextElement = document.getElementById('previous-operand');

// Initialize the calculator instance
const calculator = new Calculator(currentOperandTextElement, previousOperandTextElement);

// --- Event Listeners ---

// 1. Number and Decimal Input
numberButtons.forEach(button => {
    button.addEventListener('click', e => {
        const number = e.target.innerText;
        calculator.appendNumber(number);
        calculator.updateDisplay();
    });
});

// 2. Operator Input (+, -, *, /)
operatorButtons.forEach(button => {
    button.addEventListener('click', e => {
        const selectedOperation = e.target.innerText;
        calculator.chooseOperation(selectedOperation);
        calculator.updateDisplay();
    });
});

// 3. Equals Button (=)
equalsButton.addEventListener('click', () => {
    calculator.equals();
    calculator.updateDisplay();
});

// 4. Clear Button (AC)
clearButton.addEventListener('click', () => {
    calculator.clear();
    calculator.updateDisplay();
});

// 5. Delete/Backspace Button (DEL)
deleteButton.addEventListener('click', () => {
    calculator.delete();
    calculator.updateDisplay();
});

// Initial display update (optional, but good practice)
calculator.updateDisplay();