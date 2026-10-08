# qr-api/api/index.py
import os
import sys
import random
from flask import Flask, request, Response, jsonify
from io import BytesIO
import qrcode
from PIL import Image, ImageDraw

app = Flask(__name__)

COLOR1 = "#4ECDC4"  # Teal
COLOR2 = "#6C2BD9"  # Purple


def gradient_color(x, y, max_x, max_y, c1, c2):
    if (max_x + max_y) == 0:
        return (78, 205, 196)
    ratio = (x + y) / (max_x + max_y)
    r1, g1, b1 = tuple(int(c1[i:i + 2], 16) for i in (1, 3, 5))
    r2, g2, b2 = tuple(int(c2[i:i + 2], 16) for i in (1, 3, 5))
    r = int(r1 + (r2 - r1) * ratio)
    g = int(g1 + (g2 - g1) * ratio)
    b = int(b1 + (b2 - b1) * ratio)
    return (r, g, b)


def generate_qr(upi_id: str, amount: str, bot_name: str, remark: str) -> BytesIO:
    upi_uri = f"upi://pay?pa={upi_id}&pn={bot_name}&am={amount}&tn={remark}"

    qr = qrcode.QRCode(
        version=2,
        error_correction=qrcode.constants.ERROR_CORRECT_H,
        box_size=10,
        border=4,
    )
    qr.add_data(upi_uri)
    qr.make(fit=True)

    qr_matrix = qr.get_matrix()
    size = len(qr_matrix)
    pixel_size = 10

    img = Image.new('RGB', (size * pixel_size, size * pixel_size), 'white')
    draw = ImageDraw.Draw(img)

    for y, row in enumerate(qr_matrix):
        for x, cell in enumerate(row):
            if cell:
                color = gradient_color(x, y, size - 1, size - 1, COLOR1, COLOR2)
                draw.rectangle(
                    [(x * pixel_size, y * pixel_size),
                     ((x + 1) * pixel_size, (y + 1) * pixel_size)],
                    fill=color
                )

    bio = BytesIO()
    img.save(bio, format='PNG')
    bio.seek(0)
    return bio


@app.route('/qr', methods=['GET'])
def qr_endpoint():
    try:
        upi = request.args.get('upi')
        amount = request.args.get('amount')
        bot_name = request.args.get('bot_name')
        remark = request.args.get('remark')

        if not upi or not amount or not bot_name or not remark:
            return jsonify({
                'error': 'Missing required parameters: upi, amount, bot_name, remark'
            }), 400

        try:
            base_amount = int(float(amount))
        except (ValueError, TypeError):
            return jsonify({'error': 'Amount must be a valid number'}), 400

        if base_amount <= 0:
            return jsonify({'error': 'Amount must be positive'}), 400

        # Fingerprint: add random 1-10 paise for uniqueness
        paise = random.randint(1, 10)
        final_amount = base_amount + (paise / 100.0)
        final_amount_str = f"{final_amount:.2f}"

        qr_bytes = generate_qr(upi, final_amount_str, bot_name, remark)
        img_data = qr_bytes.read()

        response = Response(img_data, mimetype='image/png')
        response.headers['Content-Length'] = str(len(img_data))
        response.headers['X-Final-Amount'] = final_amount_str
        response.headers['X-Base-Amount'] = str(base_amount)
        response.headers['X-Paise'] = str(paise)
        response.headers['Access-Control-Expose-Headers'] = (
            'X-Final-Amount, X-Base-Amount, X-Paise'
        )
        response.headers['Cache-Control'] = 'no-store'
        return response

    except Exception as e:
        print(f"Error: {e}")
        return jsonify({'error': str(e)}), 500


@app.route('/health', methods=['GET'])
def health():
    return jsonify({'status': 'ok', 'python': sys.version})


if __name__ == '__main__':
    app.run(debug=True, port=5000)
