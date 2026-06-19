"use strict";
/**
 * Calculator module implementing core mathematical operations.
 * Handles addition, subtraction, multiplication, division, percentage, and memory functions.
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.Calculator = void 0;
class Calculator {
    constructor() {
        this.currentInput = 0;
        this.previousInput = null;
        this.operation = null;
        this.isNewCalculation = true;
    }
    /**
     * Clears all inputs and resets the calculator state.
     */
    clear() {
        this.currentInput = 0;
        this.previousInput = null;
        this.operation = null;
        this.isNewCalculation = true;
    }
    /**
     * Enters a number into the current input display.
     * @param value The number or part of the number to append.
     */
    inputDigit(value) {
        if (this.isNewCalculation) {
            this.currentInput = parseFloat(value) || 0;
            this.isNewCalculation = false;
        }
        else {
            // Handle decimal point logic
            if (value === '.' && this.currentInput.toString().includes('.')) {
                return;
            }
            this.currentInput = parseFloat(`${this.currentInput}${value}`);
        }
    }
    /**
     * Handles the operation buttons (+, -, *, /).
     * @param nextOperand The operand for the current operation.
     */
    performOperation(nextOperand) {
        if (this.operation === 'add') {
            this.currentInput += nextOperand;
        }
        else if (this.operation === 'subtract') {
            this.currentInput -= nextOperand;
        }
        else if (this.operation === 'multiply') {
            this.currentInput *= nextOperand;
        }
        else if (this.operation === 'divide') {
            if (nextOperand === 0) {
                throw new Error("Division by zero");
            }
            this.currentInput /= nextOperand;
        }
        else if (this.operation === 'equals') {
            // If equals is pressed, we finalize the calculation based on previous input
            if (this.previousInput !== null) {
                // The calculation was already done in the operation handlers, but this ensures finalization.
            }
        }
        this.previousInput = this.currentInput;
        this.isNewCalculation = true;
    }
    /**
     * Executes the pending operation (e.g., after pressing '=').
     */
    equals() {
        if (this.operation === null || this.previousInput === null)
            return;
        // If we are pressing equals, the current input becomes the second operand
        const currentVal = this.currentInput;
        let result;
        switch (this.operation) {
            case 'add':
                result = this.previousInput + currentVal;
                break;
            case 'subtract':
                result = this.previousInput - currentVal;
                break;
            case 'multiply':
                result = this.previousInput * currentVal;
                break;
            case 'divide':
                if (currentVal === 0)
                    throw new Error("Division by zero");
                result = this.previousInput / currentVal;
                break;
            default:
                return; // Should not happen
        }
        this.currentInput = result;
        this.previousInput = null;
        this.operation = null;
        this.isNewCalculation = true;
    }
    /**
     * Handles percentage calculation (e.g., 100 + 10%).
     */
    percentage() {
        this.currentInput /= 100;
        if (this.operation === 'add') { // If percentage is pressed after an operation, it usually means adding the calculated amount
            // For simplicity in this model, we treat % as dividing by 100. If it was meant to be "add percentage", the UI needs more context.
        }
    }
    /**
     * Handles memory functions (MC, MR, MS).
     */
    memoryClear() {
        // Clears the memory register (MC)
        this.previousInput = 0; // Using previousInput as the memory register for simplicity
    }
    memoryRecall() {
        // Loads memory into current input (MR)
        this.currentInput = this.previousInput !== null ? this.previousInput : 0;
        this.isNewCalculation = true;
    }
    memoryStore() {
        // Stores current input into memory (MS)
        this.previousInput = this.currentInput;
    }
    /**
     * Returns the current display value.
     */
    getCurrentValue() {
        return this.currentInput;
    }
    /**
     * Sets the current input and prepares for a new operation.
     */
    setInput(value) {
        this.currentInput = value;
        this.isNewCalculation = true;
    }
}
exports.Calculator = Calculator;
