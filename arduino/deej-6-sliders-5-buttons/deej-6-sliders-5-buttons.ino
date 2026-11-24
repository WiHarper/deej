// Based on deej-5-sliders-vanilla, extended with buttons.
// Output format: s<slider>|s<slider>|...|b<button>|b<button>|...
// Buttons use INPUT_PULLUP, so a released button sends b1 and a pressed one sends b0.

const int NUM_SLIDERS = 6;
const int analogInputs[NUM_SLIDERS] = {A0, A1, A2, A3, A4, A5};

// ASSUMPTION: change these to match the pins your buttons are wired to
const int NUM_BUTTONS = 5;
const int buttonInputs[NUM_BUTTONS] = {4, 5, 6, 7, 8};

int analogSliderValues[NUM_SLIDERS];
int buttonValues[NUM_BUTTONS];

void setup() { 
  for (int i = 0; i < NUM_SLIDERS; i++) {
    pinMode(analogInputs[i], INPUT);
  }

  for (int i = 0; i < NUM_BUTTONS; i++) {
    pinMode(buttonInputs[i], INPUT_PULLUP);
  }

  Serial.begin(9600);
}

void loop() {
  updateSliderValues();
  updateButtonValues();
  sendValues(); // Actually send data (all the time)
  // printValues(); // For debug
  delay(10);
}

void updateSliderValues() {
  for (int i = 0; i < NUM_SLIDERS; i++) {
     analogSliderValues[i] = analogRead(analogInputs[i]);
  }
}

void updateButtonValues() {
  for (int i = 0; i < NUM_BUTTONS; i++) {
     buttonValues[i] = digitalRead(buttonInputs[i]);
  }
}

void sendValues() {
  String builtString = String("");

  for (int i = 0; i < NUM_SLIDERS; i++) {
    builtString += String("s") + String((int)analogSliderValues[i]);
    builtString += String("|");
  }

  for (int i = 0; i < NUM_BUTTONS; i++) {
    builtString += String("b") + String((int)buttonValues[i]);

    if (i < NUM_BUTTONS - 1) {
      builtString += String("|");
    }
  }
  
  Serial.println(builtString);
}

void printValues() {
  for (int i = 0; i < NUM_SLIDERS; i++) {
    String printedString = String("Slider #") + String(i + 1) + String(": ") + String(analogSliderValues[i]) + String(" mV");
    Serial.write(printedString.c_str());
    Serial.write(" | ");
  }

  for (int i = 0; i < NUM_BUTTONS; i++) {
    String printedString = String("Button #") + String(i + 1) + String(": ") + String(buttonValues[i]);
    Serial.write(printedString.c_str());

    if (i < NUM_BUTTONS - 1) {
      Serial.write(" | ");
    } else {
      Serial.write("\n");
    }
  }
}
