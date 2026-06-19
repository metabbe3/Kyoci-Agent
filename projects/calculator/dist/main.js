"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const express_1 = __importDefault(require("express"));
const app = (0, express_1.default)();
const port = 3000;
// Middleware to parse JSON bodies
app.use(express_1.default.json());
// Simple root route for testing the server setup
app.get('/', (req, res) => {
    res.send('Calculator Backend Running!');
});
// TODO: Implement calculator logic routes here (e.g., POST /calculate)
app.listen(port, () => {
    console.log(`Calculator server listening at http://localhost:${port}`);
});
