// projects/calculator-website/src/main.ts

/**
 * Calculator Core Logic Implementation
 */

interface Calculation {
    operand1: number;
    operand2: number;
    operation: 'add' | 'subtract' | 'multiply' | 'divide';
}

/**
 * Performs the calculation based on the defined operation.
 * @param calc - The calculation object containing operands and operation.
 * @returns The result of the calculation, or null if division by zero occurs.
 */
export function calculate(calc: Calculation): number | null {
    switch (calc.operation) {
        case 'add':
            return calc.operand1 + calc.operand2;
        case 'subtract':
            return calc.operand1 - calc.operand2;
        case 'multiply':
            return calc.operand1 * calc.operand2;
        case 'divide':
            if (calc.operand2 === 0) {
                console.error("Error: Division by zero.");
                return null; // Handle division by zero gracefully
            }
            return calc.operand1 / calc.operand2;
        default:
            console.error("Error: Unknown operation.");
            return null;
    }
}

/**
 * Resets the calculator state.
 */
export function clear(): void {
    console.log("Calculator cleared.");
    // In a real application, this would reset the UI state variables.
}

/**
 * Handles the equals operation and returns the final result.
 * @param currentInput - The second operand or input value.
 * @param firstOperand - The initial number entered.
 * @param op - The operation to perform.
 * @returns The calculated result, or null if calculation fails.
 */
export function equals(currentInput: number, firstOperand: number, op: 'add' | 'subtract' | 'multiply' | 'divide'): number | null {
    const calculation: Calculation = {
        operand1: firstOperand,
        operand2: currentInput,
        operation: op
    };
    return calculate(calculation);
}

// Example usage (optional, for testing):
/*
const result = equals(2, 5, 'add'); // Should be 7
console.log("Result:", result);

const divisionResult = equals(10, 2, 'divide'); // Should be 5
console.log("Division Result:", divisionResult);

const errorResult = equals(10, 0, 'divide'); // Should be null
console.log("Error Result:", errorResult);
*/