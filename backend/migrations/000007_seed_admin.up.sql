-- Cria o usuário administrador padrão da plataforma.
-- Usa pgcrypto.crypt() com Blowfish (bcrypt $2a$), compatível com
-- golang.org/x/crypto/bcrypt.CompareHashAndPassword.
INSERT INTO users (email, display_name, password_hash, platform_role, email_verified_at)
VALUES (
    'gustavojucoski@gmail.com',
    'Gustavo Jucoski',
    crypt('ewq9brd5gan2dzf@FZD', gen_salt('bf', 12)),
    'platform_admin',
    NOW()
)
ON CONFLICT (email) DO NOTHING;
